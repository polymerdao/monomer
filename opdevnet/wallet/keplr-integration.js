const L2_CHAIN_CONFIG = {
    chainId: "1",
    chainName: "L2 (Monomer Devnet)",
    rpc: "http://127.0.0.1:26657",
    rest: "http://127.0.0.1:1317",
    stakeCurrency: {
        coinDenom: "ETH",
        // TODO: update coinMinimalDenom to be wei once updated in the x/rollup module
        coinMinimalDenom: "ETH",
        coinDecimals: 18,
    },
    bip44: {
        coinType: 118,
    },
    bech32Config: {
        bech32PrefixAccAddr: "cosmos",
        bech32PrefixAccPub: "cosmospub",
        bech32PrefixValAddr: "cosmosvaloper",
        bech32PrefixValPub: "cosmosvaloperpub",
        bech32PrefixConsAddr: "cosmosvalcons",
        bech32PrefixConsPub: "cosmosvalconspub",
    },
    currencies: [
        {
            coinDenom: "ETH",
            coinMinimalDenom: "ETH",
            coinDecimals: 18,
        },
    ],
    feeCurrencies: [
        {
            coinDenom: "ETH",
            coinMinimalDenom: "ETH",
            coinDecimals: 18,
        },
    ],
    gasPriceStep: {
        low: 0.01,
        average: 0.025,
        high: 0.03,
    },
}

async function runKeplrIntegration() {
    try {
        // Register the Monomer L2 chain with Keplr
        await registerChainWithKeplr();

        // Enable the Monomer L2 chain in Keplr
        await enableKeplr(L2_CHAIN_CONFIG.chainId);

        console.log("Keplr is now interacting with the Monomer L2 chain");
    } catch (err) {
        console.error("Error interacting with Keplr:", err);
    }
}

async function enableKeplr(chainId) {
    if (!window.keplr) {
        console.error("Keplr wallet is not installed");
        return;
    }
    // Enable the chain with Keplr
    await window.keplr.enable(chainId.toString());
    const offlineSigner = window.getOfflineSigner(chainId.toString());
    const accounts = await offlineSigner.getAccounts();
    console.log("Connected accounts:", accounts);
}

async function registerChainWithKeplr() {
    if (!window.keplr) {
        console.error("Keplr wallet is not installed");
        return;
    }

    try {
        await window.keplr.experimentalSuggestChain(L2_CHAIN_CONFIG);

        alert("Monomer L2 chain registered with Keplr");
    } catch (error) {
        console.error("Error registering chains:", error);
    }
}

const enableKeplrButton = document.getElementById("enable-keplr");
enableKeplrButton.addEventListener('click', runKeplrIntegration);
