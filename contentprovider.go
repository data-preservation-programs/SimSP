package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-shipyard/boostly"
	bsnetwork "github.com/ipfs/boxo/bitswap/network"
	"github.com/ipfs/boxo/bitswap/server"
	"github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/boxo/gateway"
	nilrouting "github.com/ipfs/boxo/routing/none"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/klauspost/compress/zstd"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/ybbus/jsonrpc/v3"
)

type ContentProvider struct {
	listenAddr string
	pieceMap   map[string]string // maps pieceCID to carPath
	mux        sync.RWMutex      // mutex for safe concurrent access
	multi      *multiBlockstore
	host       host.Host
	deals      map[string]MarketDeal
}

func NewContentProvider(listenAddr string, host host.Host) *ContentProvider {
	return &ContentProvider{
		listenAddr: listenAddr,
		pieceMap:   make(map[string]string),
		multi:      &multiBlockstore{},
		host:       host,
		deals:      make(map[string]MarketDeal),
	}
}

var zstdEncoder, _ = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))

func (c *ContentProvider) Start() error {
	nilRouter, err := nilrouting.ConstructNilRouting(context.Background(), nil, nil, nil)
	if err != nil {
		return errors.Wrap(err, "cannot create nil router")
	}
	net := bsnetwork.NewFromIpfsHost(c.host, nilRouter)
	serv := server.New(context.Background(), net, c.multi)
	logger.Info("Starting bitswap content provider")
	net.Start(serv)

	gw, err := gateway.NewBlocksBackend(blockservice.New(c.multi, nil))
	if err != nil {
		return errors.Wrap(err, "cannot create gateway")
	}
	gwh := gateway.NewHandler(gateway.Config{
		DeserializedResponses: true,
	}, gw)
	http.Handle("/ipfs/", gwh)
	http.HandleFunc("/piece/", c.handlePieceRequest) // handler to serve files
	http.HandleFunc("/deals", func(w http.ResponseWriter, r *http.Request) {
		c.mux.RLock()
		defer c.mux.RUnlock()
		jsn, err := json.Marshal(c.deals)
		if err != nil {
			logger.Errorf("Error marshalling deals: %v", err)
			http.Error(w, "Error marshalling deals", http.StatusInternalServerError)
			return
		}
		compressed := zstdEncoder.EncodeAll(jsn, nil)
		w.Header().Set("Content-Type", "application/zstd")
		w.Write(compressed)
	})
	logger.Infof("Starting HTTP server on %s", c.listenAddr)
	return http.ListenAndServe(c.listenAddr, nil)
}

func (c *ContentProvider) AddCar(pieceCID string, carPath string) error {
	bs, err := blockstore.OpenReadOnly(carPath)
	if err != nil {
		return errors.Wrapf(err, "cannot open car file %s", carPath)
	}
	c.multi.AddBlockstore(bs)
	c.mux.Lock()
	c.pieceMap[pieceCID] = carPath
	c.mux.Unlock()

	return nil
}

type MarketDeal struct {
	Proposal market.DealProposal
	State    DealState
}

type DealState struct {
	SectorStartEpoch int32
	LastUpdatedEpoch int32
	SlashEpoch       int32
}

func (c *ContentProvider) ActivateDeal(carDir string, deal *boostly.DealParams) error {
	lotusClient := jsonrpc.NewClient("https://api.node.glif.io/")
	var result string
	err := lotusClient.CallFor(context.TODO(),
		&result,
		"Filecoin.StateLookupID",
		deal.ClientDealProposal.Proposal.Client,
		nil)
	if err != nil {
		return errors.Wrapf(err, "failed to lookup state for wallet address %s", deal.ClientDealProposal.Proposal.Client)
	}

	short, err := address.NewFromString(result)
	if err != nil {
		return errors.Wrapf(err, "invalid actor ID %s", result)
	}

	deal.ClientDealProposal.Proposal.Client = short
	marketDeal := MarketDeal{
		Proposal: deal.ClientDealProposal.Proposal,
		State: DealState{
			SectorStartEpoch: 1,
			LastUpdatedEpoch: 1,
			SlashEpoch:       -1,
		},
	}
	dealID := fmt.Sprint(rand.Int31())
	jsn, err := json.Marshal(&marketDeal)
	if err != nil {
		return errors.Wrap(err, "cannot marshal deal")
	}
	err = os.WriteFile(filepath.Join(carDir, dealID+".json"), jsn, 0644)
	if err != nil {
		return errors.Wrap(err, "cannot write deal info file")
	}
	c.mux.Lock()
	c.deals[dealID] = marketDeal
	c.mux.Unlock()
	logger.Infof("Deal activated with deal ID %s", dealID)
	return nil
}

func (c *ContentProvider) handlePieceRequest(w http.ResponseWriter, r *http.Request) {
	pieceCID := r.URL.Path[len("/piece/"):]

	c.mux.RLock()
	carPath, exists := c.pieceMap[pieceCID]
	c.mux.RUnlock()

	if !exists {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	f, err := os.Open(carPath)
	if err != nil {
		http.Error(w, "Error opening file", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	// Obtain file info for modtime and size
	fi, err := f.Stat()
	if err != nil {
		http.Error(w, "Error obtaining file info", http.StatusInternalServerError)
		return
	}

	http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
}
