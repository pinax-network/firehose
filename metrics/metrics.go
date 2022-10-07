package metrics

import (
	"github.com/streamingfast/dmetrics"
	"time"
)

var Metricset = dmetrics.NewSet()

var AppReadiness = Metricset.NewAppReadiness("firehose")
var ActiveRequests = Metricset.NewGauge("firehose_active_requests", "Number of active requests")
var RequestCounter = Metricset.NewCounter("firehose_requests_counter", "Request count")

var ActiveSubstreams = Metricset.NewGauge("firehose_active_substreams", "Number of active substreams requests")
var SubstreamsCounter = Metricset.NewCounter("firehose_substreams_counter", "Substreams requests count")

var BlocksStreamed = Metricset.NewCounterVec("firehose_blocks_streamed", []string{"trace_id"}, "Number of blocks streamed")
var BytesStreamed = Metricset.NewCounterVec("firehose_bytes_streamed", []string{"trace_id"}, "Total size of blocks streamed")

var disconnectReason = Metricset.NewCounterVec("firehose_disconnect_reasons", []string{"reason"}, "Reasons for clients disconnecting")

func HandleDisconnect(traceID, reason string) {
	disconnectReason.Inc(reason)

	go func() {
		// wait for 2 minutes for prometheus to scrape the metrics
		// and then delete them, so we don't clutter the metrics
		time.Sleep(2 * time.Minute)
		BlocksStreamed.DeleteLabelValues(traceID)
		BytesStreamed.DeleteLabelValues(traceID)
	}()
}

// var CurrentListeners = Metricset.NewGaugeVec("current_listeners", []string{"req_type"}, "...")
// var TimedOutPushingTrxCount = Metricset.NewCounterVec("something", []string{"guarantee"}, "Number of requests for push_transaction timed out while submitting")
