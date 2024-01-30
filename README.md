# Monomer

## Background

Monomer enables ABCI applications built using Cosmos SDK to be deployed on Ethereum as a rollup.
It is currently meant to be used in conjunction with the OP stack.

## Folder Layout

`app/node/*` 
    - Chain server 
    - ABCI/CometBFT compatible RPCs
    - dbs 
`app/peptide/*`:
    - OP Stack compatible ABCI app that extends an importable ABCI app
    - Engine API RPCs
    - Payload/Tx store
`cmd/*`:
    - pepctl CLI tool for managing a peptide node
    - peptide node binary
`x/rollup/*`:
    - handles inbound L1 deposit txs and L1 block updates
