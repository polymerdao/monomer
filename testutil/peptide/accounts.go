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
$ echo -e "$mnemonic\n\n" | polymerd keys add alice -i --keyring-backend test --output json | jq

	{
		  "name": "alice",
		  "type": "local",
		  "address": "polymer158z04naus5r3vcanureh7u0ngs5q4l0g5yw8xv",
		  "pubkey": "{\"@type\":\"/cosmos.crypto.secp256k1.PubKey\",\"key\":\"AqdzCrdkG2pMSZ52Hz+ipN9W0Z0+1mMDI7sR1oLR4Qya\"}",
		  "mnemonic": "wait team asthma refuse situate crush kidney nature frown kid alpha boat engage test across cattle practice text olive level tag profit they veteran"
	}

# export --unarmored-hex prints the private hey in hex. Use xxd to decode the hex stirng and base64 to encode it
$ yes | polymerd keys export alice --keyring-backend test --unsafe --unarmored-hex | xxd -r -p | base64
5mgchgHW/twNtgLnv5y/EwU2EkkGJ0QoKTgawp8lEh0=

$ mnemonic='short eager annual love dress board buffalo enemy material awful quit analyst develop steel pave consider amazing coyote physical crew goat blind improve raven'
$ echo -e "$mnemonic\n\n" | polymerd keys add bob -i --keyring-backend test --output json | jq

	{
	  "name": "bob",
	  "type": "local",
	  "address": "polymer1l3dfk3f0vhw23qgpgt49w8q0qw6avkctharxsq",
	  "pubkey": "{\"@type\":\"/cosmos.crypto.secp256k1.PubKey\",\"key\":\"AjEJ7TVN96SUXrKz6o8UJI+RQRDfAyPV1tIQVLM4FvGu\"}",
	  "mnemonic": "short eager annual love dress board buffalo enemy material awful quit analyst develop steel pave consider amazing coyote physical crew goat blind improve raven"
	}

$ yes | polymerd keys export bob --keyring-backend test --unsafe --unarmored-hex | xxd -r -p | base64
*/
const testAccounts = `
[
  {
		"address": "polymer158z04naus5r3vcanureh7u0ngs5q4l0g5yw8xv",
		"account_number": 0,
		"sequence": 0,
		"private_key": "5mgchgHW/twNtgLnv5y/EwU2EkkGJ0QoKTgawp8lEh0="
  },
  {
		"address": "polymer1l3dfk3f0vhw23qgpgt49w8q0qw6avkctharxsq",
		"account_number": 1,
		"sequence": 0,
		"private_key": "4Hkf5sW0OyA3bbyWSClqgYQ624tZ3xKTvwXeYrWtSWA="
  },
  {
		"address": "polymer16f7llap9jplj6q7pjl7x9vhpl7mht7xahdgakk",
		"account_number": 2,
		"sequence": 0,
		"private_key": "eVv3cO5wZ8cxo7/I8jMQOaAIFEAcxADL/14+xfwUaA0="
  },
  {
		"address": "polymer1rkypctqlr8jjtwgfuyhzy8skxew02jt60k27fz",
		"account_number": 3,
		"sequence": 1,
		"private_key": "AwlXtKS5mqLrhCZnByrlppExy0hEN2YvGFzmmyMbpi0H"
  }
]`

const validatorAccounts = `
[
  {
		"address": "polymer1pqfh4m85feadw2gw8jtm40r84saqmfhscc09qu",
		"account_number": 4,
		"sequence": 1,
		"private_key": "kEZ1K8L0OV9JcA6BIHHQzwtmIh4nxSTbrc1u9YETWrc="
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
