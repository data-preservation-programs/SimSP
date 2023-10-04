# Emulated Storage Provider

This little software provides an emulated storage provider for demo purpose only.

## Features
### Deal Proposal
It accepts all boost 1.2.0 deal proposals. 

If the proposal comes with HTTP link to download CAR file, it will download the file and construct the blockstore to serve retrievals.

### HTTP Retrieval
It accepts all retrieval types, including
- IPFS Gateway
- Payload Retrieval
- Block Retrieval

### Bitswap Retrieval
It accepts bitswap retrieval but only if the block exists in downloaded CAR files.

### Retrieval Protocols
The SP will return the corresponding retrieval protocols so below commands will work
```shell
boost provider retrieval-transports f02815405
lassie fetch --provider \
  /ip4/127.0.0.1/tcp/24001/p2p/12D3KooWDeNSud283YaRmhqbZDynLNmtATBxjUPAUJxtPyEXXp9u \
  --protocols http ...
lassie fetch --provider \
  /ip4/127.0.0.1/tcp/24001/p2p/12D3KooWDeNSud283YaRmhqbZDynLNmtATBxjUPAUJxtPyEXXp9u \
  --protocols bitswap ...
```

### Index Announcement
The provider does not make advertisements to IPNI.

## How to use it 
```shell
$ ./sim-sp run -h
NAME:
   sim-sp run - Run the simulated storage provider f02815405

USAGE:
   sim-sp run [command options] [arguments...]

OPTIONS:
   --listen value [ --listen value ]  Addresses to listen on for listening for incoming storage deals or retrieval deals (default: "/ip4/0.0.0.0/tcp/24001", "/ip6/::/tcp/24001") [$SIM_SP_LISTEN]
   --key value                        Private key to use for libp2p identity encoded with base64 (default: Built-in key for 12D3KooWDeNSud283YaRmhqbZDynLNmtATBxjUPAUJxtPyEXXp9u) [$SIM_SP_KEY]
   --http value                       HTTP bind address for serving HTTP retrieval (default: ":7778") [$SIM_SP_HTTP]
   --car-dir value                    Directory to store CAR files in (default: "./cars") [$SIM_SP_CAR_DIR]
   --help, -h                         show help
```

## How to make deal with it
You just need to send deals to `f02815405` and make sure `sim-sp` is running locally.

## How does it work
`f02815405` is a real miner on the chain with a real peer ID and libp2p address. The private key and the address is provided as the default value for `--key` and `--listen` options.

When you make a deal with `f02815405`, it will lookup the multiaddr of the miner which will connect to `sim-sp` that runs locally.
