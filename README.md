# Vanity Sniper

### A simple Golang Discord vanity sniper.

## Prerequisites

* Go 1.20 or higher
* A Discord account with appropriate permissions to change the vanity URL
* A Discord server with a vanity URL

## How to Install

Clone the repo:

```
git clone https://github.com/populated/discord-vanity-sniper.git
cd discord-vanity-sniper
```

Install dependencies:

```
go mod tidy
```

Then set up your configuration data in `data/config.json` and your proxies.

And run:

```
go run sniper.go
```

![preview](https://github.com/alluding/discord-vanity-sniper/assets/149972831/82a4b9ae-ec94-4b5b-b645-7ee9ba7ea7e5)

# Future 

- [ ] Rust version?
