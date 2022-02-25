# SamlVPN

Some VPN providers allow you to log into their service using SAML.
Unfortunately, this is not a standard process and requires a custom client.
Furthermore, some providers don't distribute clients for all the operating
systems commonly used by their users. This program aims to allow you to
connect to SAML authorized VPNs from a Linux client.

## Quick Start

#### Prerequisites

- You must have a working C toolchain installed to compile OpenVPN
	- OpenVPN requires some libraries to compile. Check their docs for details.
	- Debian-based distributions can just `apt build-dep openvpn` to install dependencies.
- You must have a working Go toolchain installed to compile SamlVPN

### Compile OpenVPN

You will need to be able to compile the OpenVPN client, as it needs to be
patched. More info on this here: [OpenVPN's INSTALL
file](https://github.com/OpenVPN/openvpn/blob/master/INSTALL).

Once you are able to compile OpenVPN, checkout the `release/2.5` branch and
apply the `openvpn-v2.5.x.diff` patch to it:

```bash
git checkout release/2.5
git apply openvpn-v2.5.x.diff
```

Then, compile it again. This patch just changes some buffer sizes to allow the
much bigger SAML payloads.

Finally, you can choose to install it (`sudo make install`) or to move this
patched binary somewhere of your liking.

### Compile SamlVPN

```bash
# just compile:
make bin

# or, install to ~/go/bin
make install
```

### Usage

Once installed, you have to configure SamlVPN. It will look for the config file under:

```
$XDG_CONFIG_HOME/samlvpn/config.yaml
$XDG_CONFIG_HOME/samlvpn.yaml
$HOME/.config/samlvpn.yaml
$HOME/.samlvpn.yaml
```

Alternatively, you can specify a config file with the `-config` flag.

There is an [example config file](./config.example) with instructions on how to
configure the parameters. Carefully read it and change the values to something
that suits your usage:

```
vim ./config.example.yaml
cp ./config.example.yaml $HOME/.samlvpn.yaml
```

Finally, just run SamlVPN:

```bash
# if just compiled
./bin/samlvpn

# or, if installed
samlvpn

# if you're using run-command == false:
samlvpn | sh -C
```

### How it works

SAML VPN providers work slightly differently than regular ones.

On the first attempt to connect to the VPN, instead of using the usual
authentication procedure, the client will send `N/A` as the username, and
`ACS::{PORT}`, where `{PORT}` is a port in which the localhost will be
listening for a SAML callback. The server will then return an URL that the
localhost will need to open. This URL will start a SAML authentication
procedure, that when successful, will redirect to `localhost:{PORT}`.

This program automates the first part, then starts a server that will receive
the callback. The callback will contain the required authentication details.
At this point, a connection to the VPN is possible by using the username `N/A`
and crafting a password containing the SAML payload and some metadata from the
previous call.

### Things to keep in mind

When running this with the `runCommand` option set to `false`, the credentials
will be stored in a temporary directory (usually `/tmp`), in a file called
`openvpn-saml`. The OpenVPN command instructs OpenVPN to delete this file as
soon as the connection is established, but if OpenVPN fails to do this, the
credentials might stay there. It is for this reason that I recommend using
SamlVPN with the `runCommand` option set to `true`. When it is set to `true`,
the credentials are only stored in memory, and passed to OpenVPN through
standard input.

If you have been provided with an OpenVPN configuration file for a VPN that
requires this tool to work, it might contain non-standard configuration options
that will not be recognised by OpenVPN. You will see an error similar to
'Options error: Unrecognized option or missing or extra parameter(s) in [file]:
[configuration-option]'. Simply remove the configuration option that is causing
the error.

### Credit

This is based on [Alex Samorukov's work](github.com/samm-git/aws-vpn-client).
