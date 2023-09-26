package main

import (
	"context"
	"net/http"
	"os"
	"sync"

	"github.com/cockroachdb/errors"
	bsnetwork "github.com/ipfs/boxo/bitswap/network"
	"github.com/ipfs/boxo/bitswap/server"
	"github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/boxo/gateway"
	nilrouting "github.com/ipfs/boxo/routing/none"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/libp2p/go-libp2p/core/host"
)

type ContentProvider struct {
	listenAddr string
	pieceMap   map[string]string // maps pieceCID to carPath
	mux        sync.RWMutex      // mutex for safe concurrent access
	multi      *multiBlockstore
	host       host.Host
}

func NewContentProvider(listenAddr string, host host.Host) *ContentProvider {
	return &ContentProvider{
		listenAddr: listenAddr,
		pieceMap:   make(map[string]string),
		multi:      &multiBlockstore{},
		host:       host,
	}
}

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
