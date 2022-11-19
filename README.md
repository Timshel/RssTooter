# RssTooter

This a fork of [GoToSocial](https://github.com/superseriousbusiness/gotosocial) an [ActivityPub](https://activitypub.rocks/) social network server, written in Golang.
The goal is to patch it to allow to work as an RSS aggregator.

This is done by :
  - On a `webfinger` query if the user does not already exist try to create one using user data from Nitter
  - For all users created this way and with at least one follower poll Nitter for their timeline.


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
