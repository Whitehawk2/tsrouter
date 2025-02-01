# Tailscale Service Router

A simple Go program that creates an ephemeral Tailscale node and forwards traffic from a Tailscale IP/hostname to a local service port.
Several other solutions are available for dockerized seriveces, but I didn't use dockerized
services, and needed a reason to start learning Golang :)

## Prerequisites

- Go 1.23 or later
- A Tailscale account
- Tailscale oauth client ID and secret
- Tailscale Tailnet name (via "DNS" section of the Admin panel)

## Building

```bash
go build -ldflags "-s -w" -trimpath -o tsrouter
```

## Usage

1. Set your Tailscale variables (TS_CLIENT_ID, TS_CLIENT_ID, TS_TAILNET) as either environment variables, or in an `.env` file.

2. Run the program:

```bash
./tsrouter --hostname myservice --target-port 8080
```

### Command Line Arguments

- `--hostname`: Required. The desired Tailscale hostname for this service (will be available as hostname.your-tailnet.ts.net)
- `--target-port`: Required. The local port to forward traffic to
- `--log-level`: Optional. Set logging level (error, info, debug). Defaults to "error"

### Examples

Forward traffic to a local web service running on port 8080:

```bash
./tsrouter --hostname webui --target-port 8080
```

Forward traffic to Vaultwarden with debug logging:

```bash
./tsrouter --hostname vault --target-port 45455 --log-level debug
```

## Notes

- The program creates an ephemeral Tailscale node that will be automatically removed some time after going offline
- Multiple instances can run simultaneously to serve different services
- The service will be available across your Tailnet at `hostname.your-tailnet.ts.net`
- All traffic is forwarded over HTTPS (port 443) - first time will take a bit more time as tailscale provisions a Let's Encrypt Cert

## TODO's

This is my 1st project in Go, so there are a lot of improvements I want to make,
and some that I don't even know I need to make.

Some general improvements etc are listed in `main.go` file,
but higher level, I'd like to:

- Support dockerized version
- add CI/CD via Github Actions
- ...That will also build the app
- ... And maybe push it to some repo :)
- Nix + nix flake support :)
- MORE TO COME

In my real life I'm a DevOps engineer that just wants to learn Golang,
so fun factor and "ooh, this seems interesting" would probably play a role
in where does this project go.

## License

Yeah do whatever you want with this, I bear no responsability etc.
Mentioning this repo or my name in some form in your derivitive work
will be nice, but eh.
