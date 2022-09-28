# StreamingFast Firehose

This is a fork of StreamingFast's firehose server, which can be found [here](https://github.com/streamingfast/firehose).

## Added functionality

* Common JWT authentication by replacing the default dauth package with [this](https://github.com/pinax-network/dauth).
* Redis Pub/Sub metering by replacing the default dmetering package with [this](https://github.com/pinax-network/dmetering).

## Changed functionality

* When firehose will emit metering events it will track the `responses_count` as number of blocks emitted.