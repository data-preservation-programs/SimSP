package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/urfave/cli/v2"
)

var generatePeerCmd = &cli.Command{
	Name:  "generate-peer",
	Usage: "Generate a new peer",
	Action: func(c *cli.Context) error {
		private, public, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return errors.WithStack(err)
		}

		peerID, err := peer.IDFromPublicKey(public)
		if err != nil {
			return errors.WithStack(err)
		}

		privateBytes, err := crypto.MarshalPrivateKey(private)
		if err != nil {
			return errors.WithStack(err)
		}

		privateStr := base64.StdEncoding.EncodeToString(privateBytes)

		publicBytes, err := crypto.MarshalPublicKey(public)
		if err != nil {
			return errors.WithStack(err)
		}

		publicStr := base64.StdEncoding.EncodeToString(publicBytes)

		fmt.Println("New peer generated using ed25519, keys are encoded in base64")
		fmt.Println("peer id     : ", peerID.String())
		fmt.Println("public key  : ", publicStr)
		fmt.Println("private key : ", privateStr)
		return nil
	},
}
