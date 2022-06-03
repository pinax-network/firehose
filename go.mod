module github.com/streamingfast/firehose

go 1.15

require (
	github.com/pinax-network/dtypes v0.0.0-20220603133335-8b1cfcb0055d
	github.com/streamingfast/bstream v0.0.2-0.20220330124346-02408ab3db65
	github.com/streamingfast/dauth v0.0.0-20210812020920-1c83ba29add1
	github.com/streamingfast/dgrpc v0.0.0-20220301153539-536adf71b594
	github.com/streamingfast/dmetering v0.0.0-20220301165106-a642bb6a21bd
	github.com/streamingfast/dmetrics v0.0.0-20210811180524-8494aeb34447
	github.com/streamingfast/dstore v0.1.1-0.20220304164644-696f9c5fc231
	github.com/streamingfast/logging v0.0.0-20220304214715-bc750a74b424
	github.com/streamingfast/pbgo v0.0.6-0.20220228185940-1bbaafec7d8a
	github.com/streamingfast/shutter v1.5.0
	go.uber.org/atomic v1.9.0
	go.uber.org/zap v1.21.0
	golang.org/x/oauth2 v0.0.0-20211104180415-d3ed0bb246c8
	google.golang.org/grpc v1.44.0
	google.golang.org/protobuf v1.28.0
)

// This is required to fix build where 0.1.0 version is not considered a valid version because a v0 line does not exists
// We replace with same commit, simply tricking go and tell him that's it's actually version 0.0.3
replace github.com/census-instrumentation/opencensus-proto v0.1.0-0.20181214143942-ba49f56771b8 => github.com/census-instrumentation/opencensus-proto v0.0.3-0.20181214143942-ba49f56771b8

replace github.com/streamingfast/dauth => github.com/pinax-network/dauth v0.0.0-20220602162200-4b180cbd79e6

replace github.com/streamingfast/dmetering => github.com/pinax-network/dmetering v0.0.0-20220603141736-d210c7855f57
