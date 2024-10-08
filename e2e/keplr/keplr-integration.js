const L1_CHAIN_CONFIG = {
    chainId: "eip155:900",
    chainName: "Ethereum (Monomer Devnet)",
    rpc: "http://127.0.0.1:44601",
    rest: "http://127.0.0.1:44601",
    stakeCurrency: {
        coinDenom: "ETH",
        coinMinimalDenom: "wei",
        coinDecimals: 18,
        coinGeckoId: "ethereum",
    },
    bip44: {
        coinType: 60,
    },
    evm: {
        chainId: "900",
        rpc: "http://127.0.0.1:44601",
    },
    currencies: [
        {
            coinDenom: "ETH",
            coinMinimalDenom: "wei",
            coinDecimals: 18,
        },
    ],
    feeCurrencies: [
        {
            coinDenom: "ETH",
            coinMinimalDenom: "wei",
            coinDecimals: 18,
        },
    ],
    gasPriceStep: {
        low: 0.00000002,                // Minimum gas price (in ETH) for transactions (20 Gwei)
        average: 0.00000005,            // Average gas price (50 Gwei)
        high: 0.0000001,                // High gas price (100 Gwei)
    },
    features: ["eth-address-gen", "eth-key-sign"],
}

const L2_CHAIN_CONFIG = {
    chainId: "1",
    chainName: "L2 (Monomer Devnet)",
    rpc: "http://127.0.0.1:26657",
    rest: "http://127.0.0.1:1317",
    stakeCurrency: {
        coinDenom: "STAKE",
        coinMinimalDenom: "stake",
        coinDecimals: 6,
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
            coinDenom: "STAKE",
            coinMinimalDenom: "stake",
            coinDecimals: 6,
        },
    ],
    feeCurrencies: [
        {
            coinDenom: "STAKE",
            coinMinimalDenom: "stake",
            coinDecimals: 6,
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
        // Register the chains with Keplr
        await registerChainsWithKeplr();

        // Enable L1 and L2 chains in Keplr
        await enableKeplr(L1_CHAIN_CONFIG.chainId);
        await enableKeplr(L2_CHAIN_CONFIG.chainId);

        console.log("Keplr is now interacting with the e2e test chains");
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

async function registerChainsWithKeplr() {
    if (!window.keplr) {
        console.error("Keplr wallet is not installed");
        return;
    }

    try {
        await window.keplr.experimentalSuggestChain(L1_CHAIN_CONFIG);

        await window.keplr.experimentalSuggestChain(L2_CHAIN_CONFIG);

        alert("L1 and L2 chains registered with Keplr");
    } catch (error) {
        console.error("Error registering chains:", error);
    }
}
