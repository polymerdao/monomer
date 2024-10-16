---
draft: true
sidebar_position: 3
---

# Interact with a Monomer Rollup Chain

In this document, we will demonstrate how to interact with a Monomer rollup chain via its API endpoints.

:::note
For the most up-to-date examples of working directly with the Monomer implementation, refer to our e2e test suite in the Monomer repository.
:::

## Prerequisites

This guide assumes you have a Monomer rollup chain running locally. If you don't, refer to the [prior tutorial](./create-an-app-with-monomer.md).

## Configuring L1 and L2 Wallets

To interact with a Monomer rollup chain, you will need to configure wallets for both the L1 and L2 chains.
The L1 wallet will be used to interact with the L1 chain, submit deposit transactions to L2, and prove and finalize withdrawal transactions initiated on L2.
The L2 wallet will be used to submit transactions on the L2 chain and initiate withdrawals back to L1.

Monomer currently provides a simple wallet integration server that can automate the process of setting up wallets for both chains and depositing ETH from L1.
However, the server currently requires that MetaMask be used for the L1 wallet and Keplr for the L2 wallet.

Alternative wallets can be configured for the L1 and L2 chains, however the network configurations will need to be added manually.

:::warning
For additional safety, you should ensure that you're using a wallet specific to testing and not a wallet that stores any funds on Ethereum mainnet.
:::

To set up the wallets with the Monomer rollup chain, you can use the following command from the `monomer` repository to start the wallet integration server:

```bash
make wallet-integrations
```

Then, open up the site and follow the instructions to add the L1 wallet to MetaMask and the L2 wallet to Keplr.

## Submitting an L1 Deposit Transaction

A deposit transaction can be sent from L1 to L2 through the `OptimismPortal` contract on L1.
The wallet integration server provides a simple interface for depositing ETH from L1 to L2 where the user specifies the amount of ETH to deposit and the recipient address of the user on L2 to send the funds to.

## Submitting an L2 Cosmos SDK Transaction

L2 transactions behave the same as other Cosmos SDK chains and can be submitted to the Monomer rollup chain through the CometBFT  the Keplr wallet (or an alternative Cosmos SDK wallet if configured manually).

## Querying the Rollup Chain

The rollup chain can be queried directly through the [Cosmos SDK REST API endpoints](https://docs.cosmos.network/api#tag/Query) for supported modules.

For example, if a user wants to query the `bank` module for their account balance and is using the default API configuration, then they can use the following command:

```bash
curl http://localhost:1317/cosmos/bank/v1beta1/balances/{address}
```
