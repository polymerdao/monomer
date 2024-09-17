---
sidebar_position: 1
sidebar_label: Block Transitions
---

# Blocks

Monomer applications must support both the usual Cosmos SDK application functionality and rollup-specific workflows.

In order to acheieve this, Monomer defines custom `Block` and block `Header` types, with helper functions to generate representations that are consistent with both the Cosmos SDK and the EVM.

# State

The main application state of a Monomer application is the Cosmos Appchain state.

Supplimentary state, required for rollup bridging operations, is stored in an EVM sidecar. In principle, these requirements could be met in the Cosmos Appchain state, but this would have a knock-on effect of needing to modify some L1 components of the OP Stack. The sidecar approach allows for the reuse of existing components on both L1 and L2.

# State Transition Function

Monomer's block production function can be thought of in three phases. More detail on these processes is in the following sections.

### 1. Deposit Tx Pre-Processing

Block production is triggered by API calls from the `op-node` to Monomer. These API calls include transactions, sourced from L1, that must be included as the first transactions of the rollup block.

Because these transactions are sourced from the OP Stack and Ethereum, they must be adapted to the Cosmos SDK transaction format.

### 2. Appchain Block Production

After the deposit transactions have been adapted, they are combined with Cosmos SDK transactions from the mempool, and applied against the current state to form a new block.

### 3. Withdrawal Tx Post-Processing

The created block is inspected for withdrawal initiating transactions. Wherever an L2 transaction initiates a withdrawal, the corresponding updates are made to the EVM sidecar state via the `L2ToL1MessagePasser` contract.
