client.test("file handler", function () {
    client.assert(response.status === 200, "expected 200, got " + response.status);
});
client.log("file handler ran");
