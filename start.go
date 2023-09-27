package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/data-preservation-programs/sim-sp/model"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-shipyard/boostly"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
)

var logger = logging.Logger("sim-sp")

const yamuxID = "/yamux/1.0.0"

func yamuxTransport() network.Multiplexer {
	tpt := *yamux.DefaultTransport
	tpt.AcceptBacklog = 512
	return &tpt
}

var startCmd = &cli.Command{
	Name:  "run",
	Usage: "Run the simulated storage provider f02815405",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:    "listen",
			Usage:   "Addresses to listen on for listening for incoming storage deals or retrieval deals",
			Value:   cli.NewStringSlice("/ip4/0.0.0.0/tcp/24001", "/ip6/::/tcp/24001"),
			EnvVars: []string{"SIM_SP_LISTEN"},
		},
		&cli.StringFlag{
			Name:        "key",
			Usage:       "Private key to use for libp2p identity encoded with base64",
			Value:       "CAESQAUejksYdBAFfSKlJY5zgvOWJh/kQVrNgk73TFxMwryNON9BNYLizluGaTFx8KOT/yTTmy5ef9qOYpKS2J0EN1A=",
			DefaultText: "Built-in key for 12D3KooWDeNSud283YaRmhqbZDynLNmtATBxjUPAUJxtPyEXXp9u",
			EnvVars:     []string{"SIM_SP_KEY"},
		},
		&cli.StringFlag{
			Name:    "http",
			Usage:   "HTTP bind address for serving HTTP retrieval",
			Value:   ":7778",
			EnvVars: []string{"SIM_SP_HTTP"},
		},
		&cli.PathFlag{
			Name:    "car-dir",
			Usage:   "Directory to store CAR files in",
			Value:   "./cars",
			EnvVars: []string{"SIM_SP_CAR_DIR"},
		},
	},
	Action: func(c *cli.Context) error {
		listen := c.StringSlice("listen")
		key := c.String("key")
		carDir := c.Path("car-dir")
		http := c.String("http")
		err := os.MkdirAll(carDir, 0755)
		if err != nil {
			return errors.Wrapf(err, "cannot create car dir %s", carDir)
		}

		identityKeyBytes, err := base64.StdEncoding.DecodeString(key)
		if err != nil {
			return errors.Wrapf(err, "cannot decode identity key '%s'", key)
		}

		identityKey, err := crypto.UnmarshalPrivateKey(identityKeyBytes)
		if err != nil {
			return errors.Wrap(err, "cannot unmarshal identity private key")
		}

		peerID, err := peer.IDFromPrivateKey(identityKey)
		if err != nil {
			return errors.Wrap(err, "cannot derive peer ID from identity private key")
		}

		logger.Infof("Creating libp2p host with peer ID: %s", peerID)

		host, err := libp2p.New(
			libp2p.Identity(identityKey),
			libp2p.ListenAddrStrings(listen...),
			libp2p.Muxer(yamuxID, yamuxTransport()),
		)

		if err != nil {
			return errors.Wrap(err, "cannot create libp2p host")
		}
		defer host.Close()
		defer func() {
			<-c.Context.Done()
		}()

		contentProvider := NewContentProvider(http, host)
		entries, err := os.ReadDir(carDir)
		if err != nil {
			return errors.Wrapf(err, "cannot read car dir %s", carDir)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if !strings.HasSuffix(entry.Name(), ".car") {
				continue
			}
			pieceCID := strings.TrimSuffix(entry.Name(), ".car")
			logger.Infof("Reading car file %s with piece CID %s", entry.Name(), pieceCID)
			err = contentProvider.AddCar(pieceCID, filepath.Join(carDir, entry.Name()))
			if err != nil {
				return errors.Wrapf(err, "cannot add car file %s", entry.Name())
			}
		}

		go func() {
			err := contentProvider.Start()
			if err != nil {
				logger.Panicf("Error starting content provider: %v", err)
			}
		}()

		pendingDeals := make(chan *boostly.DealParams, 1000)

		go func() {
			for {
				select {
				case deal := <-pendingDeals:
					err := contentProvider.handleDeal(carDir, deal)
					if err != nil {
						logger.Errorf("Error handling deal: %v", err)
					}
				case <-c.Context.Done():
					return
				}
			}
		}()

		httpPort := strings.Split(http, ":")[1]
		httpAddr, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/" + httpPort + "/http")
		if err != nil {
			return errors.Wrap(err, "cannot create http multiaddr")
		}
		var bitswapAddrs []abi.Multiaddrs
		for _, addr := range listen {
			bitswapAddr, err := multiaddr.NewMultiaddr(addr)
			if err != nil {
				return errors.Wrap(err, "cannot create bitswap multiaddr")
			}
			bitswapAddrs = append(bitswapAddrs, bitswapAddr.Bytes())
		}
		host.SetStreamHandler(boostly.FilRetrievalTransportsProtocol_1_0_0, func(s network.Stream) {
			logger.Infof("Received transport protocol request from %s", s.Conn().RemotePeer())
			response := &model.QueryResponse{
				Protocols: []model.Protocol{
					{
						Name:      "http",
						Addresses: []abi.Multiaddrs{httpAddr.Bytes()},
					},
					{
						Name:      "bitswap",
						Addresses: bitswapAddrs,
					},
				},
			}

			err = cborutil.WriteCborRPC(s, response)
			if err != nil {
				logger.Errorf("Error writing response: %v", err)
			}
			s.Close()
		})

		host.SetStreamHandler(boostly.FilStorageMarketProtocol_1_2_0, func(s network.Stream) {
			logger.Infof("Received a deal proposal from %s", s.Conn().RemotePeer())
			var deal boostly.DealParams
			err := cborutil.ReadCborRPC(s, &deal)
			if err != nil {
				logger.Errorf("Error reading deal params: %v", err)
				return
			}
			logger.Infof("Deal proposal ID: %s, client: %s, pieceCID: %s",
				deal.DealUUID.String(),
				deal.ClientDealProposal.Proposal.Client.String(),
				deal.ClientDealProposal.Proposal.PieceCID.String())
			select {
			case <-c.Context.Done():
				return
			case pendingDeals <- &deal:
			}
			resp := &boostly.DealResponse{
				Accepted: true,
				Message:  "accepted",
			}
			err = cborutil.WriteCborRPC(s, resp)
			if err != nil {
				logger.Errorf("Error writing response: %v", err)
			}
		})

		return nil
	},
}

func (c *ContentProvider) handleDeal(carDir string, deal *boostly.DealParams) error {
	logger.Infof("Working on deal %s", deal.DealUUID.String())
	if deal.Transfer.Type != "http" {
		return errors.Newf("unsupported transfer type: %s", deal.Transfer.Type)
	}
	var httpRequest HttpRequest
	err := json.Unmarshal(deal.Transfer.Params, &httpRequest)
	if err != nil {
		return errors.Wrap(err, "cannot unmarshal http request")
	}

	request, err := http.NewRequest(http.MethodGet, httpRequest.URL, nil)
	for k, v := range httpRequest.Headers {
		request.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return errors.Wrap(err, "http request failed")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Newf("http request failed with status %d", resp.StatusCode)
	}

	filename := deal.DealUUID.String() + ".car.temp"
	file, err := os.Create(filepath.Join(carDir, filename))
	if err != nil {
		return errors.Wrapf(err, "cannot create car file %s", filename)
	}

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		file.Close()
		os.Remove(filepath.Join(carDir, filename))
		return errors.Wrap(err, "err encountered while downloading car file")
	}
	file.Close()
	toName := deal.ClientDealProposal.Proposal.PieceCID.String() + ".car"
	err = os.Rename(filepath.Join(carDir, filename), filepath.Join(carDir, toName))
	if err != nil {
		return errors.Wrapf(err, "cannot rename car file from %s to %s", filename, toName)
	}

	logger.Infof("Deal %s completed successfully", deal.DealUUID.String())

	pieceCID := deal.ClientDealProposal.Proposal.PieceCID.String()
	logger.Infof("Reading car file %s with piece CID %s", toName, pieceCID)
	err = c.AddCar(pieceCID, filepath.Join(carDir, toName))
	if err != nil {
		return errors.Wrapf(err, "cannot add car file %s", toName)
	}
	return nil
}

type HttpRequest struct {
	URL     string
	Headers map[string]string
}
