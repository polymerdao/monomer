package peptest

import (
	"encoding/json"

	"github.com/polymerdao/monomer/app/peptide"
)

// NOTE use these hardcoded accounts for the node and testing until we get proper account support
var (
	Accounts          peptide.SignerAccounts
	ValidatorAccounts peptide.SignerAccounts
)

/*
These accounts are used by the op-relayer. They are account 0 and 1 from the list below.
Here's how they were generated

$ mnemonic='wait team asthma refuse situate crush kidney nature frown kid alpha boat engage test across cattle practice text olive level tag profit they veteran'
$ echo -e "$mnemonic\n\n" | gm keys add alice -i --keyring-backend test --output json | jq

	{
		  "name": "alice",
		  "type": "local",
		  "address": "cosmos158z04naus5r3vcanureh7u0ngs5q4l0gujga27",
		  "pubkey": "{\"@type\":\"/cosmos.crypto.secp256k1.PubKey\",\"key\":\"AqdzCrdkG2pMSZ52Hz+ipN9W0Z0+1mMDI7sR1oLR4Qya\"}",
		  "mnemonic": "wait team asthma refuse situate crush kidney nature frown kid alpha boat engage test across cattle practice text olive level tag profit they veteran"
	}

# export --unarmored-hex prints the private hey in hex. Use xxd to decode the hex stirng and base64 to encode it
$ yes | gm keys export alice --keyring-backend test --unsafe --unarmored-hex | xxd -r -p | base64
5mgchgHW/twNtgLnv5y/EwU2EkkGJ0QoKTgawp8lEh0=

$ mnemonic='short eager annual love dress board buffalo enemy material awful quit analyst develop steel pave consider amazing coyote physical crew goat blind improve raven'
$ echo -e "$mnemonic\n\n" | gm keys add bob -i --keyring-backend test --output json | jq

	{
	  "name": "bob",
	  "type": "local",
	  "address": "cosmos1l3dfk3f0vhw23qgpgt49w8q0qw6avkctlt9uuj",
	  "pubkey": "{\"@type\":\"/cosmos.crypto.secp256k1.PubKey\",\"key\":\"AjEJ7TVN96SUXrKz6o8UJI+RQRDfAyPV1tIQVLM4FvGu\"}",
	  "mnemonic": "short eager annual love dress board buffalo enemy material awful quit analyst develop steel pave consider amazing coyote physical crew goat blind improve raven"
	}

$ yes | gm keys export bob --keyring-backend test --unsafe --unarmored-hex | xxd -r -p | base64
4Hkf5sW0OyA3bbyWSClqgYQ624tZ3xKTvwXeYrWtSWA=

$ mnemonic='oblige banner flush coconut mushroom rescue become this legal whisper oblige giraffe drink smoke mystery near legal dignity ignore eternal dial era pizza require'
$ echo -e "$mnemonic\n\n" | gm keys add joe -i --keyring-backend test --output json | jq

	{
	  "name": "joe",
	  "type": "local",
	  "address": "cosmos1gn9y70kyhaeyxq7qnh53tccf0h8z62j4zfxa68",
	  "pubkey": "{\"@type\":\"/cosmos.crypto.secp256k1.PubKey\",\"key\":\"AsYgIEHH3RGCLB9m2w9sm7//RoRSPJYDQghnCWXPA6mW\"}",
	  "mnemonic": "oblige banner flush coconut mushroom rescue become this legal whisper oblige giraffe drink smoke mystery near legal dignity ignore eternal dial era pizza require"
	}

$ yes | gm keys export joe --keyring-backend test --unsafe --unarmored-hex | xxd -r -p | base64
qMxoP6VQjzh4KKOU9eL/teg/4TNAkYsATwRqPnzKtU4=
*/
const testAccounts = `
[
  {
		"address": "cosmos158z04naus5r3vcanureh7u0ngs5q4l0gujga27",
		"account_number": 0,
		"sequence": 0,
		"private_key": "5mgchgHW/twNtgLnv5y/EwU2EkkGJ0QoKTgawp8lEh0="
  },
  {
		"address": "cosmos1l3dfk3f0vhw23qgpgt49w8q0qw6avkctlt9uuj",
		"account_number": 1,
		"sequence": 0,
		"private_key": "4Hkf5sW0OyA3bbyWSClqgYQ624tZ3xKTvwXeYrWtSWA="
  }
]`

const validatorAccounts = `
[
  {
		"address": "cosmos1gn9y70kyhaeyxq7qnh53tccf0h8z62j4zfxa68",
		"account_number": 4,
		"sequence": 1,
		"private_key": "qMxoP6VQjzh4KKOU9eL/teg/4TNAkYsATwRqPnzKtU4="
  }
]`

func init() {
	if err := json.Unmarshal([]byte(testAccounts), &Accounts); err != nil {
		panic(err)
	}
	if err := json.Unmarshal([]byte(validatorAccounts), &ValidatorAccounts); err != nil {
		panic(err)
	}
}
