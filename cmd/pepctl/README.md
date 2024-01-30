# pepctl

`pepctl` is a command line tool for managing Peptide node, its db stores, and potentially other resources.
It's intended to be used by Peptide Node operators.

## Installation

**Install on host OS**

`just install`

**Install in Cloud**

```sh
just build-linux
kubectl cp pepctl default/peptide-0:/root/.peptide/
```

## Fix [#987](https://github.com/polymerdao/polymerase/issues/987)

**Inspect DB stores**

```sh
~/.peptide  ./pepctl db inspect

I[2024-01-03|22:19:31.061] Peptide db inspect                           home=/root/.peptide chain-id=901 db-backend=goleveldb db-override=false
I[2024-01-03|22:19:31.061] Genesis                                      error=null time="2023-12-15 00:27:08 +0000 UTC" initalHeight=0
I[2024-01-03|22:19:31.061] GenesisBlock                                 height=1 hash=0xa8255e505af475cfd76c6b3c88afe742f20d0204a6f04a2adf548f46ddc33060
I[2024-01-03|22:19:31.061] Genesis L1                                   height=4887791 hash=0xfa30a985938bfe2a494d7397493d35059d0822139972c854731f35c0ae276d79
I[2024-01-03|22:19:31.061] App state                                    appLastBlockHeight=541810 ChainId=901 BondDenom=stake
I[2024-01-03|22:19:31.071] Block store at app.lastBlockHeight()         blockExists=true hash=0x1bbdef11d8fa6df93e8c56ec4482b3275bd3be4a0e1e80e6356d1b9546f9b4a0
I[2024-01-03|22:19:31.074] Block store height by labels                 latest/unsafe=null safe=541810 finalized=541810 appLastBlockHeight=541810
E[2024-01-03|22:19:31.074] Stores sanity check                          error="rollback height must be set: <nil>"
Error: rollback height must be set: <nil>

```

As we can see above, BlockStore's latest/unsafe is null, indicating a corrupted state. Safe and finalized block heights
are the same as app.lastBlockHeight, which is expected, as op-node had advanced L2 heads even after Peptide halted.

Unsafe head in general should be the same as app.lastBlockHeight. We need to set unsafe head poiniter in BlockStore to `app.lastBlockHeight`.

**Rollback DB stores**

```sh
# dry run to check if rollback is possible
~/.peptide  ./pepctl db rollback -r 541810 -u 541810 -s 541810 -f 541810 -n --override
I[2024-01-03|22:28:26.028] Peptide db rollback                          home=/root/.peptide chain-id=901 db-backend=goleveldb db-override=true
I[2024-01-03|22:28:26.028] RollbackSetting                              rollbackHeight=541810 unsafeHeight=541810 safeHeight=541810 finalizedHeight=541810
I[2024-01-03|22:28:26.029] Rollback setting OK. Dry run success

# rollback updates BlockStore and app state
~/.peptide ./pepctl db rollback -r 541810 -u 541810 -s 541810 -f 541810  --override
I[2024-01-03|22:29:20.659] Peptide db rollback                          home=/root/.peptide chain-id=901 db-backend=goleveldb db-override=true
I[2024-01-03|22:29:20.659] RollbackSetting                              rollbackHeight=541810 unsafeHeight=541810 safeHeight=541810 finalizedHeight=541810
I[2024-01-03|22:30:04.211] Rollback success
I[2024-01-03|22:30:04.211] Block store height by labels                 latest/unsafe=541810 safe=541810 finalized=541810 appLastBlockHeight=541810

```

**Reset chain App latest version/height**

NOTE: This is dangerous op!

It's only used in special scenarios when Peptide hit OOM during a reorg and App is in the middle of rollback and ends up
in a corrupted state where IVAL nodeDB's implicit latest version is not the same as App's explicit latest version.

This cmd force reset App's explicit latest version to match IVAL nodeDB's implicit latest version.

```sh
./pepctl db reset-app -l 562394 -n
I[2024-01-06|01:25:13.608] Peptide db reset-app                         home=/root/.peptide latest-version=562394 dry-run=true
I[2024-01-06|01:25:13.665] Existing latest version                      version=562560
~/.peptide # ./pepctl db reset-app -l 562394
I[2024-01-06|01:25:23.018] Peptide db reset-app                         home=/root/.peptide latest-version=562394 dry-run=false
I[2024-01-06|01:25:23.070] Existing latest version                      version=562560
I[2024-01-06|01:25:23.071] Reset app state success                      latest-version=562394
~/.peptide # ./pepctl db rollback -r 562394 -u 562394 -s 562394 -f 562394 --override
I[2024-01-06|01:25:49.686] Peptide db rollback                          home=/root/.peptide chain-id=901 db-backend=goleveldb db-override=true
I[2024-01-06|01:25:49.686] RollbackSetting                              rollbackHeight=562394 unsafeHeight=562394 safeHeight=562394 finalizedHeight=562394
I[2024-01-06|01:39:08.799] Rollback success
I[2024-01-06|01:39:08.803] Block store height by labels                 latest/unsafe=562394 safe=562394 finalized=562394 appLastBlockHeight=562394

```

## Test

**Generate Peptide blocks**

Run `pepctl standalone --home ~/.peptide-standalone` in a terminal for a few seconds, then press `Ctrl+C` to stop it.

**Check Peptide stores**

We can inspect Peptide stores state from any Peptide home directory.
Run `pepctl db inspect --home ~/.peptide-standalone`
Or `just inspect`

Inspect is read-only operation. Peptide node must be stopped before running any db related cmds.

**Rollback Peptide sotres to a previous height**

Run `pepctl db rollback --home ~/.peptide-standalone --override -r 10 -s 10 -u 10 -f 5`,

or `just r=10 u=10 s=10 f=5 rollback`

**NOTE**: Rollback is a write operation. Please backup your Peptide home directory before rollback.

## Manual

### Inspect Peptide stores

```sh
pepctl db inspect -h
Inspect Peptide db

Usage:
  pepctl db inspect [flags]

Aliases:
  inspect, i

Flags:
      --db-backend string   Database backend type. Supported types: goleveldb, cleveldb, rocksdb, badgerdb, boltdb, memdb (default "goleveldb")
  -h, --help                help for inspect
      --home string         Home directory. Defaults to: ~/.peptide (default "~/.peptide")
      --override            Overrides any existing storage in the homedir
```

### Rollback Peptide stores

```sh
pepctl db rollback -h
Rollback stores to a given height with integrity check

Usage:
  pepctl db rollback [flags]

Aliases:
  rollback, rb

Flags:
      --db-backend string     Database backend type. Supported types: goleveldb, cleveldb, rocksdb, badgerdb, boltdb, memdb (default "goleveldb")
  -n, --dry-run               Dry run without updating stores; check if rollback is possible
  -f, --finalized int         Block height of finalized block (must be <= safe)
  -h, --help                  help for rollback
      --home string           Home directory. Defaults to: ~/.peptide (default "~/.peptide")
      --override              Overrides any existing storage in the homedir
  -r, --rollback-height int   Rollback BlockStore/AppState to height
  -s, --safe int              Block height of safe block (must be <= unsafe)
  -u, --unsafe int            Block height of unsafe block (must be <= height)
```

### Reset App latest version

```sh
./pepctl db reset-app -h
NOTE: onlyuse this cmd when App state is corrupted. Otherwise try `pepctl db rollback

Usage:
  pepctl db reset-app [flags]

Aliases:
  reset-app, ra

Flags:
      --db-backend string    Database backend type. Supported types: goleveldb, cleveldb, rocksdb, badgerdb, boltdb, memdb (default "goleveldb")
  -n, --dry-run              Dry run mode without mutation
  -h, --help                 help for reset-app
      --home string          Home directory. Defaults to: /root/.peptide (default "/root/.peptide")
  -l, --latest-version int   Latest version of the corrupted app state
      --override             Overrides any existing storage in the homedir

```
