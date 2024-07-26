// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.25;

/// @title L2ApplicationStateRootProvider
/// @notice The L2ApplicationStateRootProvider stores the state root of the Cosmos application using Monomer.
///         This is necessary for implementing withdrawals because we need a valid account proof for the
///         L2ToL1MessagePasser via the top-level Monomer state root.
contract L2ApplicationStateRootProvider {
    /// @notice The current state root of the Cosmos application using Monomer.
    bytes32 public l2ApplicationStateRoot;

    /// @notice Stores the current state root of the Cosmos application using Monomer.
    /// @param _l2ApplicationStateRoot The new state root of the Cosmos application using Monomer.
    function setL2ApplicationStateRoot(bytes32 _l2ApplicationStateRoot) public {
        l2ApplicationStateRoot = _l2ApplicationStateRoot;
    }
}
