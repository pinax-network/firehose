// Copyright 2020 dfuse Platform Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package firehose

import (
	"context"
	"fmt"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/blockstream"
	"github.com/streamingfast/bstream/hub"
	"github.com/streamingfast/bstream/transform"
	dauth "github.com/streamingfast/dauth/authenticator"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/firehose"
	"github.com/streamingfast/firehose/metrics"
	"github.com/streamingfast/firehose/server"
	"github.com/streamingfast/shutter"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type Config struct {
	BlockStoreURLs                  []string // Blocks store
	IrreversibleBlocksIndexStoreURL string
	IrreversibleBlocksBundleSizes   []uint64
	BlockStreamAddr                 string        // gRPC endpoint to get real-time blocks, can be "" in which live streams is disabled
	GRPCListenAddr                  string        // gRPC address where this app will listen to
	GRPCShutdownGracePeriod         time.Duration // The duration we allow for gRPC connections to terminate gracefully prior forcing shutdown
	RealtimeTolerance               time.Duration
}

type RegisterServiceExtensionFunc func(firehoseServer *server.Server, streamFactory *firehose.StreamFactory, logger *zap.Logger)

type Modules struct {
	// Required dependencies
	Authenticator            dauth.Authenticator
	HeadTimeDriftMetric      *dmetrics.HeadTimeDrift
	HeadBlockNumberMetric    *dmetrics.HeadBlockNum
	Tracker                  *bstream.Tracker
	TransformRegistry        *transform.Registry
	RegisterServiceExtension RegisterServiceExtensionFunc
}

type App struct {
	*shutter.Shutter
	config  *Config
	modules *Modules
	logger  *zap.Logger

	isReady *atomic.Bool
}

func New(logger *zap.Logger, config *Config, modules *Modules) *App {
	return &App{
		Shutter: shutter.New(),
		config:  config,
		modules: modules,
		logger:  logger,

		isReady: atomic.NewBool(false),
	}
}

func (a *App) Run() error {
	dmetrics.Register(metrics.Metricset)
	appCtx, cancel := context.WithCancel(context.Background())
	a.Shutter.OnTerminating(func(_ error) {
		cancel()
	})

	a.logger.Info("running firehose", zap.Reflect("config", a.config))
	if err := a.config.Validate(); err != nil {
		return fmt.Errorf("invalid app config: %w", err)
	}

	blockStores := make([]dstore.Store, len(a.config.BlockStoreURLs))
	for i, url := range a.config.BlockStoreURLs {
		store, err := dstore.NewDBinStore(url)
		if err != nil {
			return fmt.Errorf("failed setting up block store from url %q: %w", url, err)
		}

		blockStores[i] = store
	}

	var store dstore.Store
	if url := a.config.IrreversibleBlocksIndexStoreURL; url != "" {
		var err error
		store, err = dstore.NewStore(url, "", "", false)
		if err != nil {
			return fmt.Errorf("failed setting up irreversible blocks index store from url %q: %w", url, err)
		}
	}

	withLive := a.config.BlockStreamAddr != ""

	var subscriptionHub *hub.SubscriptionHub
	var serverLiveSourceFactory bstream.SourceFactory
	var serverLiveHeadTracker bstream.BlockRefGetter

	if withLive {
		var err error
		subscriptionHub, err = a.newSubscriptionHub(blockStores)
		if err != nil {
			return fmt.Errorf("setting up subscription hub: %w", err)
		}

		serverLiveHeadTracker = subscriptionHub.HeadTracker
		serverLiveSourceFactory = bstream.SourceFactory(func(h bstream.Handler) bstream.Source {
			return subscriptionHub.NewSource(h, 250)
		})
	}

	a.logger.Info("creating gRPC server", zap.Bool("live_support", withLive))

	streamFactory := firehose.NewStreamFactory(
		blockStores,
		store,
		a.config.IrreversibleBlocksBundleSizes,
		serverLiveSourceFactory,
		serverLiveHeadTracker,
		a.modules.Tracker,
		a.modules.TransformRegistry,
	)

	server := server.New(
		a.modules.TransformRegistry,
		streamFactory,
		a.logger,
		a.modules.Authenticator,
		a.IsReady,
		a.config.GRPCListenAddr,
	)
	a.OnTerminating(func(_ error) { server.Shutdown(a.config.GRPCShutdownGracePeriod) })
	server.OnTerminated(a.Shutdown)

	if withLive {
		// get subscriptionHub  StartBlock   server.modules.tracker appCtx
		var start uint64
		a.logger.Info("retrieving live start block")
		for retries := 0; ; retries++ {
			lib, err := a.modules.Tracker.Get(appCtx, bstream.BlockStreamLIBTarget)
			if err != nil {
				if retries%5 == 4 {
					a.logger.Warn("cannot get lib num from blockstream, retrying", zap.Int("retries", retries), zap.Error(err))
				}
				time.Sleep(time.Second)
				continue
			}
			head, err := a.modules.Tracker.Get(appCtx, bstream.BlockStreamHeadTarget)
			if err != nil {
				a.logger.Info("firehose hub: tracker cannot get HEAD block number, rewinding start block further", zap.Error(err))
				start = previousBundle(lib.Num())
				break
			}

			if head.Num()/100 == lib.Num()/100 {
				start = previousBundle(lib.Num())
				a.logger.Info("firehose hub: tracker HEAD is in same bundle as LIB, rewinding start block further",
					zap.Uint64("head", head.Num()),
					zap.Uint64("lib", lib.Num()),
					zap.Uint64("start", start),
				)
				break
			}
			start = lib.Num()
			break
		}
		go subscriptionHub.LaunchAt(start)
	}

	if a.modules.RegisterServiceExtension != nil {
		a.modules.RegisterServiceExtension(server, streamFactory, a.logger)
	}

	go server.Launch()

	if withLive {
		// Blocks app startup until ready
		a.logger.Info("waiting until hub is real-time synced")
		subscriptionHub.WaitUntilRealTime(appCtx)
	}

	a.logger.Info("firehose is now ready to accept request")
	a.isReady.CAS(false, true)

	return nil
}

func previousBundle(in uint64) uint64 {
	if in <= 100 {
		return in
	}
	out := (in - 100) / 100 * 100 // round down
	if out < bstream.GetProtocolFirstStreamableBlock {
		return bstream.GetProtocolFirstStreamableBlock
	}
	return out
}

func (a *App) newSubscriptionHub(blockStores []dstore.Store) (*hub.SubscriptionHub, error) {

	liveSourceFactory := bstream.SourceFromNumFactory(func(startBlockNum uint64, h bstream.Handler) bstream.Source {
		return blockstream.NewSource(
			context.Background(),
			a.config.BlockStreamAddr,
			100,
			bstream.HandlerFunc(func(blk *bstream.Block, obj interface{}) error {
				a.modules.HeadBlockNumberMetric.SetUint64(blk.Num())
				a.modules.HeadTimeDriftMetric.SetBlockTime(blk.Time())

				return h.ProcessBlock(blk, obj)
			}),
			blockstream.WithRequester("firehose"),
		)
	})

	fileSourceFactory := bstream.SourceFromNumFactory(func(startBlockNum uint64, h bstream.Handler) bstream.Source {
		var options []bstream.FileSourceOption
		if len(blockStores) > 1 {
			options = append(options, bstream.FileSourceWithSecondaryBlocksStores(blockStores[1:]))
		}

		a.logger.Info("creating file source", zap.String("block_store", blockStores[0].ObjectPath("")), zap.Uint64("start_block_num", startBlockNum))
		src := bstream.NewFileSource(blockStores[0], startBlockNum, 1, nil, h, options...)
		return src
	})

	a.logger.Info("setting up subscription hub")
	buffer := bstream.NewBuffer("hub-buffer", a.logger.Named("hub"))
	tailManager := bstream.NewSimpleTailManager(buffer, 350)
	go tailManager.Launch()

	return hub.NewSubscriptionHub(
		0, // we override this later with LaunchAt
		buffer,
		tailManager.TailLock,
		fileSourceFactory,
		liveSourceFactory,
		hub.Withlogger(a.logger),
		hub.WithRealtimeTolerance(a.config.RealtimeTolerance),
	)
}

// IsReady return `true` if the apps is ready to accept requests, `false` is returned
// otherwise.
func (a *App) IsReady(ctx context.Context) bool {
	if a.IsTerminating() {
		return false
	}

	return a.isReady.Load()
}

// Validate inspects itself to determine if the current config is valid according to
// Firehose rules.
func (config *Config) Validate() error {
	return nil
}
