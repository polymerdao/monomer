# install spawn, pinned at v0.50.5
git clone https://github.com/rollchains/spawn
cd spawn
git checkout v0.50.5
make install

# Bootstrap CosmosSDK applicaion
spawn new rollchain --consensus=proof-of-authority \
    --bech32=roll \
    --denom=uroll \
    --bin=rolld \
    --disabled=cosmwasm,globalfee,block-explorer \
    --org=monomer

# Integrating Monomer

cd rollchain
git apply ../monomer.patch
git add .
git commit -m "Integrate Monomer"

# Build the application
make install # check if the application builds successfully

