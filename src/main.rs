use std::str::FromStr;

use anyhow::{Context, Result};
use chrono::{SecondsFormat, Utc};
use clap::ArgAction;

use crate::error::Error::{ResponseBody, SendRequest};

mod error;
mod form;
mod model;
mod parse;

fn main() -> Result<()> {
    let cmd = clap::Command::new("httper")
        .arg(
            clap::Arg::new("file")
                .help("File containing the HTTP request")
                .required(true),
        )
        .arg(
            clap::Arg::new("verbose")
                .action(ArgAction::SetTrue)
                .short('v')
                .long("verbose")
                .help("Print verbose output"),
        )
        .arg(
            clap::Arg::new("output")
                .short('o')
                .long("output")
                .value_name("FILE")
                .help("Output file for the response"),
        );

    let matches = cmd.get_matches();
    let filepath = matches.get_one::<String>("file").unwrap();
    let output = matches.get_one::<String>("output");
    let verbose = matches.get_flag("verbose");

    let content =
        std::fs::read_to_string(filepath).context(format!("cannot open file at: {}", filepath))?;

    let full_path = std::env::current_dir()?.join(filepath);
    let directory = full_path.parent().unwrap().to_str().unwrap();

    let client = reqwest::blocking::ClientBuilder::new()
        .connection_verbose(verbose)
        .use_rustls_tls()
        .danger_accept_invalid_certs(true)
        .build()?;

    let requests = parse::parse_requests(content.as_str(), client.clone(), directory)?;

    for request in requests {
        send_one(request, &client, output, verbose).unwrap();
    }

    Ok(())
}

fn send_one(
    request: reqwest::blocking::Request,
    client: &reqwest::blocking::Client,
    output: Option<&String>,
    verbose: bool,
) -> Result<()> {
    if verbose {
        println!("\n{:?}", request);
        let body = request.body();
        if body.is_some() {
            println!("{:?}", body.unwrap());
        }
        println!("{}", "-".repeat(80));
    }

    let start = std::time::Instant::now();
    let response = client.execute(request).map_err(SendRequest)?;

    let duration = start.elapsed();

    let headers = response.headers().clone();
    let status_code = response.status();
    let content_length = response.content_length();
    let bytes = response.bytes().map_err(ResponseBody)?;

    let content_type = headers
        .iter()
        .filter_map(|(k, v)| {
            if k != reqwest::header::CONTENT_TYPE {
                return None;
            }

            let header_value = v.to_str().unwrap_or_default();
            if [
                mime::APPLICATION_OCTET_STREAM.as_ref(),
                mime::TEXT_PLAIN_UTF_8.as_ref(),
                mime::TEXT_PLAIN.as_ref(),
            ]
            .contains(&header_value)
            {
                return None;
            }

            mime::Mime::from_str(header_value).ok()
        })
        .collect::<Vec<_>>();

    // todo consider disposition header here maybe?

    if let Some(content_type) = content_type.first() {
        let extensions = mime_guess::get_mime_extensions(content_type);

        if extensions.is_some() {
            let extension = extensions.unwrap().first().unwrap();

            if verbose {
                println!("Content type: {:?}", content_type);
                println!("Extension: {:?}", extension);
            }

            let filename = if let Some(output) = output {
                output.to_string()
            } else {
                format!(
                    "response-{}.{}",
                    Utc::now().to_rfc3339_opts(SecondsFormat::Secs, true),
                    extension
                )
            };

            if let Err(e) = std::fs::write(filename, bytes.clone()) {
                eprintln!("Failed to write response to file: {}", e);
            }
        }
    }

    let content_length = content_length.unwrap_or(bytes.len() as u64);

    if verbose {
        println!("Headers: {:?}", headers);
        if let Some(content_type) = content_type.first() {
            if !content_type.to_string().starts_with("image") {
                println!("Content: {}", String::from_utf8_lossy(&bytes));
            }
            println!("Content type: {:?}", content_type);
        }
    }

    println!(
        "\nResponse code: {}; Time: {}ms ({:?}); Content length: {} bytes ({:.2} MB)",
        status_code,
        duration.as_millis(),
        duration,
        content_length,
        content_length as f64 / 1_000_000.0,
    );

    Ok(())
}
