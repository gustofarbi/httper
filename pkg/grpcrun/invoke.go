package grpcrun

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jhump/protoreflect/v2/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Options carries the per-request knobs for Invoke.
type Options struct {
	Insecure  bool                // skip TLS verification on TLS targets
	Timeout   time.Duration       // whole-call deadline including reflection; 0 = none
	Metadata  metadata.MD         // outgoing request metadata
	OnMessage func(message []byte) // called with each response message's JSON as it arrives
}

// Outcome is the result of a completed RPC. A non-OK status is an Outcome,
// not an error — mirroring how HTTP non-2xx responses are reported.
type Outcome struct {
	Code     codes.Code
	Message  string // status message for non-OK codes
	Header   metadata.MD
	Trailer  metadata.MD
	Messages [][]byte // response messages as compact JSON
	// Streaming marks a server-streaming method, where Messages is a stream
	// rather than the single unary response.
	Streaming bool
	Duration  time.Duration
}

// Invoke dials the target, resolves the method via server reflection, sends
// bodyJSON as the request message, and collects the response message(s).
// Unary and server-streaming methods are supported. Errors are reserved for
// failures outside the RPC itself (dial, reflection, unknown method,
// unsupported call type, body marshalling).
func Invoke(ctx context.Context, target Target, bodyJSON string, opts Options) (*Outcome, error) {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	conn, err := grpc.NewClient(target.Address, grpc.WithTransportCredentials(transportCredentials(target, opts)))
	if err != nil {
		return nil, fmt.Errorf("dialing %s: %w", target.Address, err)
	}
	defer func() { _ = conn.Close() }()

	reflectionClient := grpcreflect.NewClientAuto(ctx, conn)
	defer reflectionClient.Reset()

	method, err := resolveMethod(reflectionClient, target)
	if err != nil {
		return nil, err
	}

	resolver := reflectionClient.AsResolver().AsTypeResolver()

	request := dynamicpb.NewMessage(method.Input())
	if bodyJSON != "" {
		unmarshal := protojson.UnmarshalOptions{Resolver: resolver}
		if err := unmarshal.Unmarshal([]byte(bodyJSON), request); err != nil {
			return nil, fmt.Errorf("request body for %s: %w", target.FullMethod, err)
		}
	}

	marshal := protojson.MarshalOptions{Resolver: resolver}
	ctx = metadata.NewOutgoingContext(ctx, opts.Metadata)
	outcome := &Outcome{}

	outcome.Streaming = method.IsStreamingServer()

	start := time.Now()
	if method.IsStreamingServer() {
		err = invokeServerStream(ctx, conn, target, method, request, marshal, opts.OnMessage, outcome)
	} else {
		err = invokeUnary(ctx, conn, target, method, request, marshal, opts.OnMessage, outcome)
	}
	outcome.Duration = time.Since(start)

	if err != nil {
		rpcStatus, ok := status.FromError(err)
		if !ok {
			return nil, err
		}
		outcome.Code = rpcStatus.Code()
		outcome.Message = rpcStatus.Message()
	}

	return outcome, nil
}

func transportCredentials(target Target, opts Options) credentials.TransportCredentials {
	if target.PlainText {
		return insecure.NewCredentials()
	}

	// #nosec G402 -- explicit user opt-in via the -insecure flag
	return credentials.NewTLS(&tls.Config{InsecureSkipVerify: opts.Insecure})
}

func resolveMethod(client *grpcreflect.Client, target Target) (protoreflect.MethodDescriptor, error) {
	descriptor, err := client.AsResolver().FindDescriptorByName(protoreflect.FullName(target.Service))
	if err != nil {
		return nil, fmt.Errorf("resolving service %s via reflection: %w", target.Service, err)
	}
	service, ok := descriptor.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("%s is not a service", target.Service)
	}

	method := service.Methods().ByName(protoreflect.Name(target.Method))
	if method == nil {
		return nil, fmt.Errorf("service %s has no method %s", target.Service, target.Method)
	}
	if method.IsStreamingClient() {
		return nil, fmt.Errorf("%s: client/bidirectional streaming is not supported", target.FullMethod)
	}

	return method, nil
}

func invokeUnary(
	ctx context.Context,
	conn *grpc.ClientConn,
	target Target,
	method protoreflect.MethodDescriptor,
	request *dynamicpb.Message,
	marshal protojson.MarshalOptions,
	onMessage func([]byte),
	outcome *Outcome,
) error {
	response := dynamicpb.NewMessage(method.Output())

	err := conn.Invoke(ctx, target.FullMethod, request, response,
		grpc.Header((*metadata.MD)(&outcome.Header)),
		grpc.Trailer((*metadata.MD)(&outcome.Trailer)),
	)
	if err != nil {
		return err
	}

	return appendMessage(response, marshal, onMessage, outcome)
}

func invokeServerStream(
	ctx context.Context,
	conn *grpc.ClientConn,
	target Target,
	method protoreflect.MethodDescriptor,
	request *dynamicpb.Message,
	marshal protojson.MarshalOptions,
	onMessage func([]byte),
	outcome *Outcome,
) error {
	description := &grpc.StreamDesc{StreamName: target.Method, ServerStreams: true}
	stream, err := conn.NewStream(ctx, description, target.FullMethod)
	if err != nil {
		return err
	}
	if err := stream.SendMsg(request); err != nil {
		return err
	}
	if err := stream.CloseSend(); err != nil {
		return err
	}

	if header, err := stream.Header(); err == nil {
		outcome.Header = header
	}

	for {
		response := dynamicpb.NewMessage(method.Output())
		if err := stream.RecvMsg(response); err != nil {
			outcome.Trailer = stream.Trailer()
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if err := appendMessage(response, marshal, onMessage, outcome); err != nil {
			return err
		}
	}
}

func appendMessage(
	response *dynamicpb.Message,
	marshal protojson.MarshalOptions,
	onMessage func([]byte),
	outcome *Outcome,
) error {
	rendered, err := marshal.Marshal(response)
	if err != nil {
		return fmt.Errorf("rendering response message: %w", err)
	}

	outcome.Messages = append(outcome.Messages, rendered)
	if onMessage != nil {
		onMessage(rendered)
	}

	return nil
}
