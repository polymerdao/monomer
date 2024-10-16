if (typeof window.ethereum !== 'undefined') {
    console.log('Metamask is installed');
} else {
    alert('Please install Metamask');
}

const enableMetamaskButton = document.getElementById('enable-metamask');
const accountSpan = document.getElementById('account');
const statusParagraph = document.getElementById('status');
const ethDepositButton = document.getElementById('eth-deposit-button');
const ethAmountInput = document.getElementById('eth-amount');
const ethRecipientInput = document.getElementById('eth-recipient');
const erc20DepositButton = document.getElementById('erc20-deposit-button');
const erc20AmountInput = document.getElementById('erc20-amount');
const erc20RecipientInput = document.getElementById('erc20-recipient');
const erc20AddressInput = document.getElementById('erc20-token-addr');

let optimismPortalContract;
let l1StandardBridgeContract;

const optimismPortalAbi = [{"type":"constructor","inputs":[],"stateMutability":"nonpayable"},{"type":"receive","stateMutability":"payable"},{"type":"function","name":"depositTransaction","inputs":[{"name":"_to","type":"address","internalType":"address"},{"name":"_value","type":"uint256","internalType":"uint256"},{"name":"_gasLimit","type":"uint64","internalType":"uint64"},{"name":"_isCreation","type":"bool","internalType":"bool"},{"name":"_data","type":"bytes","internalType":"bytes"}],"outputs":[],"stateMutability":"payable"},{"type":"function","name":"donateETH","inputs":[],"outputs":[],"stateMutability":"payable"},{"type":"function","name":"finalizeWithdrawalTransaction","inputs":[{"name":"_tx","type":"tuple","internalType":"structTypes.WithdrawalTransaction","components":[{"name":"nonce","type":"uint256","internalType":"uint256"},{"name":"sender","type":"address","internalType":"address"},{"name":"target","type":"address","internalType":"address"},{"name":"value","type":"uint256","internalType":"uint256"},{"name":"gasLimit","type":"uint256","internalType":"uint256"},{"name":"data","type":"bytes","internalType":"bytes"}]}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"finalizedWithdrawals","inputs":[{"name":"","type":"bytes32","internalType":"bytes32"}],"outputs":[{"name":"","type":"bool","internalType":"bool"}],"stateMutability":"view"},{"type":"function","name":"guardian","inputs":[],"outputs":[{"name":"","type":"address","internalType":"address"}],"stateMutability":"view"},{"type":"function","name":"initialize","inputs":[{"name":"_l2Oracle","type":"address","internalType":"contractL2OutputOracle"},{"name":"_systemConfig","type":"address","internalType":"contractSystemConfig"},{"name":"_superchainConfig","type":"address","internalType":"contractSuperchainConfig"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"isOutputFinalized","inputs":[{"name":"_l2OutputIndex","type":"uint256","internalType":"uint256"}],"outputs":[{"name":"","type":"bool","internalType":"bool"}],"stateMutability":"view"},{"type":"function","name":"l2Oracle","inputs":[],"outputs":[{"name":"","type":"address","internalType":"contractL2OutputOracle"}],"stateMutability":"view"},{"type":"function","name":"l2Sender","inputs":[],"outputs":[{"name":"","type":"address","internalType":"address"}],"stateMutability":"view"},{"type":"function","name":"minimumGasLimit","inputs":[{"name":"_byteCount","type":"uint64","internalType":"uint64"}],"outputs":[{"name":"","type":"uint64","internalType":"uint64"}],"stateMutability":"pure"},{"type":"function","name":"params","inputs":[],"outputs":[{"name":"prevBaseFee","type":"uint128","internalType":"uint128"},{"name":"prevBoughtGas","type":"uint64","internalType":"uint64"},{"name":"prevBlockNum","type":"uint64","internalType":"uint64"}],"stateMutability":"view"},{"type":"function","name":"paused","inputs":[],"outputs":[{"name":"paused_","type":"bool","internalType":"bool"}],"stateMutability":"view"},{"type":"function","name":"proveWithdrawalTransaction","inputs":[{"name":"_tx","type":"tuple","internalType":"structTypes.WithdrawalTransaction","components":[{"name":"nonce","type":"uint256","internalType":"uint256"},{"name":"sender","type":"address","internalType":"address"},{"name":"target","type":"address","internalType":"address"},{"name":"value","type":"uint256","internalType":"uint256"},{"name":"gasLimit","type":"uint256","internalType":"uint256"},{"name":"data","type":"bytes","internalType":"bytes"}]},{"name":"_l2OutputIndex","type":"uint256","internalType":"uint256"},{"name":"_outputRootProof","type":"tuple","internalType":"structTypes.OutputRootProof","components":[{"name":"version","type":"bytes32","internalType":"bytes32"},{"name":"stateRoot","type":"bytes32","internalType":"bytes32"},{"name":"messagePasserStorageRoot","type":"bytes32","internalType":"bytes32"},{"name":"latestBlockhash","type":"bytes32","internalType":"bytes32"}]},{"name":"_withdrawalProof","type":"bytes[]","internalType":"bytes[]"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"provenWithdrawals","inputs":[{"name":"","type":"bytes32","internalType":"bytes32"}],"outputs":[{"name":"outputRoot","type":"bytes32","internalType":"bytes32"},{"name":"timestamp","type":"uint128","internalType":"uint128"},{"name":"l2OutputIndex","type":"uint128","internalType":"uint128"}],"stateMutability":"view"},{"type":"function","name":"superchainConfig","inputs":[],"outputs":[{"name":"","type":"address","internalType":"contractSuperchainConfig"}],"stateMutability":"view"},{"type":"function","name":"systemConfig","inputs":[],"outputs":[{"name":"","type":"address","internalType":"contractSystemConfig"}],"stateMutability":"view"},{"type":"function","name":"version","inputs":[],"outputs":[{"name":"","type":"string","internalType":"string"}],"stateMutability":"view"},{"type":"event","name":"Initialized","inputs":[{"name":"version","type":"uint8","indexed":false,"internalType":"uint8"}],"anonymous":false},{"type":"event","name":"TransactionDeposited","inputs":[{"name":"from","type":"address","indexed":true,"internalType":"address"},{"name":"to","type":"address","indexed":true,"internalType":"address"},{"name":"version","type":"uint256","indexed":true,"internalType":"uint256"},{"name":"opaqueData","type":"bytes","indexed":false,"internalType":"bytes"}],"anonymous":false},{"type":"event","name":"WithdrawalFinalized","inputs":[{"name":"withdrawalHash","type":"bytes32","indexed":true,"internalType":"bytes32"},{"name":"success","type":"bool","indexed":false,"internalType":"bool"}],"anonymous":false},{"type":"event","name":"WithdrawalProven","inputs":[{"name":"withdrawalHash","type":"bytes32","indexed":true,"internalType":"bytes32"},{"name":"from","type":"address","indexed":true,"internalType":"address"},{"name":"to","type":"address","indexed":true,"internalType":"address"}],"anonymous":false},{"type":"error","name":"BadTarget","inputs":[]},{"type":"error","name":"CallPaused","inputs":[]},{"type":"error","name":"GasEstimation","inputs":[]},{"type":"error","name":"LargeCalldata","inputs":[]},{"type":"error","name":"OutOfGas","inputs":[]},{"type":"error","name":"SmallGasLimit","inputs":[]}];
const optimismPortalAddress = '0x9A676e781A523b5d0C0e43731313A708CB607508';
const l1StandardBridgeAbi = [{"type":"constructor","inputs":[],"stateMutability":"nonpayable"},{"type":"receive","stateMutability":"payable"},{"type":"function","name":"MESSENGER","inputs":[],"outputs":[{"name":"","type":"address","internalType":"contractCrossDomainMessenger"}],"stateMutability":"view"},{"type":"function","name":"OTHER_BRIDGE","inputs":[],"outputs":[{"name":"","type":"address","internalType":"contractStandardBridge"}],"stateMutability":"view"},{"type":"function","name":"bridgeERC20","inputs":[{"name":"_localToken","type":"address","internalType":"address"},{"name":"_remoteToken","type":"address","internalType":"address"},{"name":"_amount","type":"uint256","internalType":"uint256"},{"name":"_minGasLimit","type":"uint32","internalType":"uint32"},{"name":"_extraData","type":"bytes","internalType":"bytes"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"bridgeERC20To","inputs":[{"name":"_localToken","type":"address","internalType":"address"},{"name":"_remoteToken","type":"address","internalType":"address"},{"name":"_to","type":"address","internalType":"address"},{"name":"_amount","type":"uint256","internalType":"uint256"},{"name":"_minGasLimit","type":"uint32","internalType":"uint32"},{"name":"_extraData","type":"bytes","internalType":"bytes"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"bridgeETH","inputs":[{"name":"_minGasLimit","type":"uint32","internalType":"uint32"},{"name":"_extraData","type":"bytes","internalType":"bytes"}],"outputs":[],"stateMutability":"payable"},{"type":"function","name":"bridgeETHTo","inputs":[{"name":"_to","type":"address","internalType":"address"},{"name":"_minGasLimit","type":"uint32","internalType":"uint32"},{"name":"_extraData","type":"bytes","internalType":"bytes"}],"outputs":[],"stateMutability":"payable"},{"type":"function","name":"depositERC20","inputs":[{"name":"_l1Token","type":"address","internalType":"address"},{"name":"_l2Token","type":"address","internalType":"address"},{"name":"_amount","type":"uint256","internalType":"uint256"},{"name":"_minGasLimit","type":"uint32","internalType":"uint32"},{"name":"_extraData","type":"bytes","internalType":"bytes"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"depositERC20To","inputs":[{"name":"_l1Token","type":"address","internalType":"address"},{"name":"_l2Token","type":"address","internalType":"address"},{"name":"_to","type":"address","internalType":"address"},{"name":"_amount","type":"uint256","internalType":"uint256"},{"name":"_minGasLimit","type":"uint32","internalType":"uint32"},{"name":"_extraData","type":"bytes","internalType":"bytes"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"depositETH","inputs":[{"name":"_minGasLimit","type":"uint32","internalType":"uint32"},{"name":"_extraData","type":"bytes","internalType":"bytes"}],"outputs":[],"stateMutability":"payable"},{"type":"function","name":"depositETHTo","inputs":[{"name":"_to","type":"address","internalType":"address"},{"name":"_minGasLimit","type":"uint32","internalType":"uint32"},{"name":"_extraData","type":"bytes","internalType":"bytes"}],"outputs":[],"stateMutability":"payable"},{"type":"function","name":"deposits","inputs":[{"name":"","type":"address","internalType":"address"},{"name":"","type":"address","internalType":"address"}],"outputs":[{"name":"","type":"uint256","internalType":"uint256"}],"stateMutability":"view"},{"type":"function","name":"finalizeBridgeERC20","inputs":[{"name":"_localToken","type":"address","internalType":"address"},{"name":"_remoteToken","type":"address","internalType":"address"},{"name":"_from","type":"address","internalType":"address"},{"name":"_to","type":"address","internalType":"address"},{"name":"_amount","type":"uint256","internalType":"uint256"},{"name":"_extraData","type":"bytes","internalType":"bytes"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"finalizeBridgeETH","inputs":[{"name":"_from","type":"address","internalType":"address"},{"name":"_to","type":"address","internalType":"address"},{"name":"_amount","type":"uint256","internalType":"uint256"},{"name":"_extraData","type":"bytes","internalType":"bytes"}],"outputs":[],"stateMutability":"payable"},{"type":"function","name":"finalizeERC20Withdrawal","inputs":[{"name":"_l1Token","type":"address","internalType":"address"},{"name":"_l2Token","type":"address","internalType":"address"},{"name":"_from","type":"address","internalType":"address"},{"name":"_to","type":"address","internalType":"address"},{"name":"_amount","type":"uint256","internalType":"uint256"},{"name":"_extraData","type":"bytes","internalType":"bytes"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"finalizeETHWithdrawal","inputs":[{"name":"_from","type":"address","internalType":"address"},{"name":"_to","type":"address","internalType":"address"},{"name":"_amount","type":"uint256","internalType":"uint256"},{"name":"_extraData","type":"bytes","internalType":"bytes"}],"outputs":[],"stateMutability":"payable"},{"type":"function","name":"initialize","inputs":[{"name":"_messenger","type":"address","internalType":"contractCrossDomainMessenger"},{"name":"_superchainConfig","type":"address","internalType":"contractSuperchainConfig"}],"outputs":[],"stateMutability":"nonpayable"},{"type":"function","name":"l2TokenBridge","inputs":[],"outputs":[{"name":"","type":"address","internalType":"address"}],"stateMutability":"view"},{"type":"function","name":"messenger","inputs":[],"outputs":[{"name":"","type":"address","internalType":"contractCrossDomainMessenger"}],"stateMutability":"view"},{"type":"function","name":"otherBridge","inputs":[],"outputs":[{"name":"","type":"address","internalType":"contractStandardBridge"}],"stateMutability":"view"},{"type":"function","name":"paused","inputs":[],"outputs":[{"name":"","type":"bool","internalType":"bool"}],"stateMutability":"view"},{"type":"function","name":"superchainConfig","inputs":[],"outputs":[{"name":"","type":"address","internalType":"contractSuperchainConfig"}],"stateMutability":"view"},{"type":"function","name":"version","inputs":[],"outputs":[{"name":"","type":"string","internalType":"string"}],"stateMutability":"view"},{"type":"event","name":"ERC20BridgeFinalized","inputs":[{"name":"localToken","type":"address","indexed":true,"internalType":"address"},{"name":"remoteToken","type":"address","indexed":true,"internalType":"address"},{"name":"from","type":"address","indexed":true,"internalType":"address"},{"name":"to","type":"address","indexed":false,"internalType":"address"},{"name":"amount","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"extraData","type":"bytes","indexed":false,"internalType":"bytes"}],"anonymous":false},{"type":"event","name":"ERC20BridgeInitiated","inputs":[{"name":"localToken","type":"address","indexed":true,"internalType":"address"},{"name":"remoteToken","type":"address","indexed":true,"internalType":"address"},{"name":"from","type":"address","indexed":true,"internalType":"address"},{"name":"to","type":"address","indexed":false,"internalType":"address"},{"name":"amount","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"extraData","type":"bytes","indexed":false,"internalType":"bytes"}],"anonymous":false},{"type":"event","name":"ERC20DepositInitiated","inputs":[{"name":"l1Token","type":"address","indexed":true,"internalType":"address"},{"name":"l2Token","type":"address","indexed":true,"internalType":"address"},{"name":"from","type":"address","indexed":true,"internalType":"address"},{"name":"to","type":"address","indexed":false,"internalType":"address"},{"name":"amount","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"extraData","type":"bytes","indexed":false,"internalType":"bytes"}],"anonymous":false},{"type":"event","name":"ERC20WithdrawalFinalized","inputs":[{"name":"l1Token","type":"address","indexed":true,"internalType":"address"},{"name":"l2Token","type":"address","indexed":true,"internalType":"address"},{"name":"from","type":"address","indexed":true,"internalType":"address"},{"name":"to","type":"address","indexed":false,"internalType":"address"},{"name":"amount","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"extraData","type":"bytes","indexed":false,"internalType":"bytes"}],"anonymous":false},{"type":"event","name":"ETHBridgeFinalized","inputs":[{"name":"from","type":"address","indexed":true,"internalType":"address"},{"name":"to","type":"address","indexed":true,"internalType":"address"},{"name":"amount","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"extraData","type":"bytes","indexed":false,"internalType":"bytes"}],"anonymous":false},{"type":"event","name":"ETHBridgeInitiated","inputs":[{"name":"from","type":"address","indexed":true,"internalType":"address"},{"name":"to","type":"address","indexed":true,"internalType":"address"},{"name":"amount","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"extraData","type":"bytes","indexed":false,"internalType":"bytes"}],"anonymous":false},{"type":"event","name":"ETHDepositInitiated","inputs":[{"name":"from","type":"address","indexed":true,"internalType":"address"},{"name":"to","type":"address","indexed":true,"internalType":"address"},{"name":"amount","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"extraData","type":"bytes","indexed":false,"internalType":"bytes"}],"anonymous":false},{"type":"event","name":"ETHWithdrawalFinalized","inputs":[{"name":"from","type":"address","indexed":true,"internalType":"address"},{"name":"to","type":"address","indexed":true,"internalType":"address"},{"name":"amount","type":"uint256","indexed":false,"internalType":"uint256"},{"name":"extraData","type":"bytes","indexed":false,"internalType":"bytes"}],"anonymous":false},{"type":"event","name":"Initialized","inputs":[{"name":"version","type":"uint8","indexed":false,"internalType":"uint8"}],"anonymous":false}]
const l1StandardBridgeAddress = '0x959922bE3CAee4b8Cd9a407cc3ac1C251C2007B1';

async function connectMetamask() {
    try {
        // Request account access if needed
        await window.ethereum.request({ method: 'eth_requestAccounts' });
        let provider = new ethers.providers.Web3Provider(window.ethereum);
        let signer = provider.getSigner();
        accountSpan.innerText = await signer.getAddress();

        optimismPortalContract = new ethers.Contract(
            optimismPortalAddress,
            optimismPortalAbi,
            signer
        );

        l1StandardBridgeContract = new ethers.Contract(
            l1StandardBridgeAddress,
            l1StandardBridgeAbi,
            signer
        );

        console.log('Connected to MetaMask and contract instantiated.');
    } catch (error) {
        console.error('Error connecting to MetaMask:', error);
    }
}

async function addL1ChainToMetaMask() {
    try {
        await window.ethereum.request({
            method: 'wallet_addEthereumChain',
            params: [{
                chainId: '0x384', // 900
                chainName: 'Ethereum (Monomer Devnet)',
                nativeCurrency: {
                    name: 'ETH',
                    symbol: 'ETH',
                    decimals: 18
                },
                rpcUrls: ['http://127.0.0.1:9001'],
                blockExplorerUrls: []
            }]
        });
        statusParagraph.innerText = 'L1 chain has been added to Metamask!';
        console.log('Ethereum (Monomer Devnet) added successfully.');
    } catch (error) {
        if (error.code === 4001) {
            console.error('User rejected the request to add the network.');
            statusParagraph.innerText = 'Request to add network rejected by user.';
        } else {
            console.error('Failed to add the network:', error);
            statusParagraph.innerText = `Error: ${error.message}`;
        }
    }
}

async function depositETH() {
    let amount = ethAmountInput.value;
    if (!amount || isNaN(amount) || Number(amount) <= 0) {
        statusParagraph.innerText = 'Please enter a valid amount.';
        return;
    }
    amount = ethers.utils.parseEther(amount);

    let recipient = ethRecipientInput.value;
    if (!recipient) {
        statusParagraph.innerText = 'Please enter a valid Cosmos recipient address.';
        return;
    }

    try {
        const ethRecipient = cosmosToEthAddress(recipient);
        if (!ethRecipient) {
            statusParagraph.innerText = 'Invalid Cosmos recipient address.';
            return;
        }

        statusParagraph.innerText = 'Sending transaction...';

        const tx = await optimismPortalContract.depositTransaction(
            ethRecipient, // To
            amount,       // Value
            500000,       // Gas limit
            false,        // isCreation
            '0x',         // Data
            {
                value: amount // Mint
            }
        );

        statusParagraph.innerText = `Transaction sent: ${tx.hash}`;
        console.log('Transaction details:', tx);

        // Wait for transaction confirmation
        await tx.wait();
        statusParagraph.innerText = 'Transaction confirmed!';
        console.log('Transaction confirmed.');
    } catch (error) {
        console.error('Error depositing ETH:', error);
        statusParagraph.innerText = `Error: ${error.message}`;
    }
}

async function depositERC20() {
    let amount = erc20AmountInput.value;
    if (!amount || isNaN(amount) || Number(amount) <= 0) {
        statusParagraph.innerText = 'Please enter a valid amount.';
        return;
    }
    amount = ethers.utils.parseEther(amount);

    let recipient = erc20RecipientInput.value;
    if (!recipient) {
        statusParagraph.innerText = 'Please enter a valid Cosmos recipient address.';
        return;
    }

    let tokenAddress = erc20AddressInput.value;
    if (!tokenAddress) {
        statusParagraph.innerText = 'Please enter a valid ERC-20 token address.';
        return;
    }

    try {
        const ethRecipient = cosmosToEthAddress(recipient);
        if (!ethRecipient) {
            statusParagraph.innerText = 'Invalid Cosmos recipient address.';
            return;
        }

        statusParagraph.innerText = 'Sending transaction...';

        const tx = await l1StandardBridgeContract.depositERC20To(
            tokenAddress, // L1 Token Address
            tokenAddress, // L2 Token Address
            recipient,    // Recipient
            amount,       // Amount
            1000000,      // Min Gas limit
            '0x',         // Data
        );

        statusParagraph.innerText = `Transaction sent: ${tx.hash}`;
        console.log('Transaction details:', tx);

        // Wait for transaction confirmation
        await tx.wait();
        statusParagraph.innerText = 'Transaction confirmed!';
        console.log('Transaction confirmed.');
    } catch (error) {
        console.error('Error depositing ETH:', error);
        statusParagraph.innerText = `Error: ${error.message}`;
    }
}

// TODO: do we need to update cosmosToEthAddress if the conversion logic changes?
function cosmosToEthAddress(cosmosAddress) {
    // Decode the Cosmos Bech32 address
    const decoded = bech32.decode(cosmosAddress);
    const bytes = bech32.fromWords(decoded.words);

    // Convert the byte array to a hex string (EVM address)
    const ethAddress = '0x' + bytes.map(byte => byte.toString(16).padStart(2, '0')).join('');

    if (ethAddress.length !== 42) {
        throw new Error('Invalid length for Ethereum address');
    }

    console.log('Converted Cosmos address to Ethereum address:', cosmosAddress, ethAddress);

    return ethAddress;
}

ethDepositButton.addEventListener('click', depositETH);
erc20DepositButton.addEventListener('click', depositERC20);
enableMetamaskButton.addEventListener('click', addL1ChainToMetaMask);

// Connect to Metamask on page load
window.addEventListener('load', () => {
    connectMetamask();
});
