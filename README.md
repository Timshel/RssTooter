# RssTooter

This a fork of [GoToSocial](https://github.com/superseriousbusiness/gotosocial) an [ActivityPub](https://activitypub.rocks/) social network server, written in Golang.
The goal is to patch it to allow to work as an RSS aggregator.

This is done by :
  - On a `webfinger` query if the user does not already exist try to create one using user data from Nitter
  - For all users created this way will start polling every `rss-poll-frequency`.

## Webfinger query

Since Mastodon support a limited char range (ATM [[a-z0-9_]+([a-z0-9_\.]+[a-z0-9_]+)?](https://github.com/mastodon/mastodon/blob/d8c428472356abd70aaf1f514b99114464ee7f61/app/models/account.rb#L70)) it's not possible to directly pass the url through Mastodon.

The idea is to call the webfinger directly using `curl` for example, the endpoint will return a username that can be used in mastodon.

The winfinger will:
 - Remove the `@server_host` if present
 - Check if the lefovers are an existing account name.
 - Add `https` protocol if needed and call the resulting url to check if it's a Feed (`Atom` or `Rss`).
 - If not load the page HTML and search for:
    - A `link` element in the header with `type="application/atom+xml"`
    - A `link` element in the header with `type="application/rss+xml"`

The returned user will be either a pretified version of the url if short enough or the host appended with an [xxhash](https://github.com/cespare/xxhash) of the query path and parameters.

Ex:
```bash
Mastodon
> curl 'http://127.0.0.1:8080/.well-known/webfinger?resource=xkcd.com'
{
    "subject": "acct:xkcd.com@127.0.0.1:8080",
    "aliases":
    [
        "http://127.0.0.1:8080/users/xkcd.com",
        "https://xkcd.com/atom.xml"
    ],
    ...
}
> curl 'http://127.0.0.1:8080/.well-known/webfinger?resource=codeberg.org/forgejo/forgejo/releases.rss'
{
    "subject": "acct:codeberg.org.14499953154663570662@127.0.0.1:8080",
    "aliases":
    [
        "http://127.0.0.1:8080/users/codeberg.org.14499953154663570662",
        "https://codeberg.org/forgejo/forgejo/releases.rss"
    ],
    ...
}
> curl 'http://127.0.0.1:8080/.well-known/webfinger?resource=@codeberg.org.14499953154663570662@127.0.0.1:8080'
{
    "subject": "acct:codeberg.org.14499953154663570662@127.0.0.1:8080",
    ...
}
> curl 'http://127.0.0.1:8080/.well-known/webfinger?resource=codeberg.org.14499953154663570662'
{
    "subject": "acct:codeberg.org.14499953154663570662@127.0.0.1:8080",
    ...
}
```


## Setup

Note: since the goal is to make minimum change to the project to be able to continue updating the `gotosocial` base, the package was not renammed.

```bash
mkdir -p ~/go/src/github.com/superseriousbusiness
git clone https://github.com/Timshel/napp.git ~/go/src/github.com/superseriousbusiness/gotosocial
cd ~/go/src/github.com/superseriousbusiness/gotosocial
yarn install --cwd web/source
yarn --cwd ./web/source build

./scripts/build.sh

cp example/config.yaml config.yaml
./gotosocial --config-path ./config.yaml server start
./gotosocial --config-path ./config.yaml admin account create --username admin --email toto@yopmail.com --password 'Password'
./gotosocial --config-path ./config.yaml admin account confirm --username admin
./gotosocial --config-path ./config.yaml admin account promote --username admin
./gotosocial --config-path ./config.yaml server start
```


Systemd example file:

```systemd
[Unit]
Description=RssTooter
After=syslog.target
After=network.target

[Service]
Type=simple

# set user and group
User=rssTooter
Group=rssTooter

# configure location
WorkingDirectory=/opt/rssTooter/live
ExecStart=/opt/rssTooter/live/gotosocial --config-path ./config.yaml server start

Restart=always
RestartSec=15

[Install]
WantedBy=multi-user.target
```
