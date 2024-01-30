# Gm Tutorial 

Git clone ignite cli repo + make install v0.27.2.
```
git clone git@github.com:ignite/cli.git
cd cli/
git checkout v0.27.2
make install
```

This installs a versioned ignite binary to your go bin folder.

Scaffold a gm chain:
```
ignite scaffold chain github.com/YOURUSER/gm -p gm
```

Push to your repo above and import your ABCI app instead of `github.com/notbdu/gm/app` inside of `app/peptide/app.go`.
