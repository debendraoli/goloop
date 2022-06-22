package sync2

import (
	"time"

	"github.com/icon-project/goloop/common/codec"
	"github.com/icon-project/goloop/common/db"
	"github.com/icon-project/goloop/common/log"
	"github.com/icon-project/goloop/common/merkle"
	"github.com/icon-project/goloop/module"
	"github.com/icon-project/goloop/service/state"
)

const (
	configSyncPriority      = 3
	configExpiredTime       = 500             // in millisecond
	configMaxExpiredTime    = 1200            // in millisecond
	configDiscoveryInterval = 2 * time.Second // second
)

var c = codec.MP

type RequestCallback func(ver byte, dataLen int, id module.PeerID)

type Syncer interface {
	GetBuilder(accountsHash, pReceiptsHash, nReceiptsHash, validatorListHash, extensionData []byte) merkle.Builder
	SyncWithBuilders(buildersV1 []merkle.Builder, buildersV2 []merkle.Builder) (*Result, error)
	Stop()
	Finalize() error
}

type SyncReactor interface {
	ExistReadyPeer() bool
	GetVersion() byte
	GetPeers() []*peer
}

type Platform interface {
	NewExtensionWithBuilder(builder merkle.Builder, raw []byte) state.ExtensionSnapshot
}

type Manager struct {
	log log.Logger

	db       db.Database
	plt      Platform
	syncer   Syncer
	ds       *dataSyncer
	reactors []SyncReactor
	builders []merkle.Builder
}

type Result struct {
	Wss            state.WorldSnapshot
	PatchReceipts  module.ReceiptList
	NormalReceipts module.ReceiptList
	// BPTData
}

func (m *Manager) GetSyncer() Syncer {
	if m.syncer == nil {
		syncer := newSyncer(m.db, m.reactors, m.plt, m.log)
		m.syncer = syncer
	}

	return m.syncer
}

func (m *Manager) GetSyncBuilders(ah, prh, nrh, vlh, ed []byte) []merkle.Builder {
	builder := m.syncer.GetBuilder(ah, prh, nrh, vlh, ed)
	m.builders = append(m.builders, builder)

	return m.builders
}

func (m *Manager) AddRequest(id db.BucketID, key []byte) error {
	return m.ds.AddRequest(id, key)
}

func (m *Manager) Start() {
	m.ds.Start()
}

func (m *Manager) Term() {
	m.ds.Term()
}

func NewSyncManager(database db.Database, nm module.NetworkManager, plt Platform, logger log.Logger) *Manager {
	logger = logger.WithFields(log.Fields{log.FieldKeyModule: "statesync2"})
	logger.Debugln("NewSyncManager")
	m := new(Manager)

	reactorV1 := newReactorV1(database, logger, protoV1)
	ph, err := nm.RegisterReactorForStreams("statesync", module.ProtoStateSync, reactorV1, protocol, configSyncPriority, module.NotRegisteredProtocolPolicyClose)
	if err != nil {
		logger.Panicf("Failed to register reactorV1 for stateSync\n")
		return nil
	}
	reactorV1.ph = ph
	m.reactors = append(m.reactors, reactorV1)

	reactorV2 := newReactorV2(database, logger, protoV2)
	pi2 := module.NewProtocolInfo(module.ProtoStateSync.ID(), 1)
	ph2, err := nm.RegisterReactorForStreams("statesync2", pi2, reactorV2, protocolv2, configSyncPriority, module.NotRegisteredProtocolPolicyClose)
	if err != nil {
		logger.Panicf("Failed to register reactorV2 for stateSync2\n")
		return nil
	}
	reactorV2.ph = ph2
	m.reactors = append(m.reactors, reactorV2)

	m.db = database
	m.plt = plt
	m.log = logger

	m.ds = newDataSyncer(m.db, m.reactors, logger)

	syncer := newSyncer(m.db, m.reactors, m.plt, m.log)
	m.syncer = syncer

	return m
}
