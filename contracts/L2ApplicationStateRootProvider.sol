// SPDX-License-Identifier: MIT
pragma solidity 0.8.25;

contract L2ApplicationStateRootProvider {
    uint256 public l2ApplicationStateRoot;

    function setL2ApplicationStateRoot(uint256 _l2ApplicationStateRoot) public {
        l2ApplicationStateRoot = _l2ApplicationStateRoot;
    }
}
