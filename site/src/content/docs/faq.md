---
title: "Frequently asked questions"
---

- [Why is it called Yagi?](#why-is-it-called-yagi)
- [How do I say it?](#how-do-i-say-it)
- [It used to be called Antennarr. What do I need to do?](#it-used-to-be-called-antennarr-what-do-i-need-to-do)
- [Does the viewer need to install anything?](#does-the-viewer-need-to-install-anything)
- [Does it send anything over the internet?](#does-it-send-anything-over-the-internet)
- [Is it free?](#is-it-free)
- [Can I share my tuner with the public?](#can-i-share-my-tuner-with-the-public)

## Why is it called Yagi?

The rooftop TV antenna, a boom with a row of rods across it, is a Yagi antenna. It is named for Hidetsugu Yagi, the Japanese engineer who, with his colleague Shintaro Uda at Tohoku Imperial University, developed the design in the 1920s, and Yagi's English-language publications made it known worldwide. Properly it is the Yagi–Uda antenna.

Yagi the program does something similar for a tuner: it takes a signal that only reaches one place and points it at the people you choose. The name is a nod to the inventors, and the logo is the antenna. And, purely by accident, the admin page's default port, 9244, spells YAGI on a phone keypad.

## How do I say it?

"YAH-gee", the Japanese way, which is also how ham radio operators say it. "YAG-ee" will be understood.

## It used to be called Antennarr. What do I need to do?

Nothing right away, and nothing breaks:

- **The web page.** Old `/antennarr/` addresses and bookmarks redirect to `/`. The one-time setup link in an old log is only good until you restart, so use the new one.
- **Your settings file.** If you start Yagi without `-config` and there is an `antennarr.json` but no `yagi.json`, it uses the old file and says so in the log. Renaming the file to `yagi.json` (and updating any `-config` you pass, such as in a compose file) stops the message.
- **The container image.** New releases are published as `ghcr.io/johnschult/yagi`, and for a few releases also as `ghcr.io/johnschult/antennarr` with the same tags, so an existing compose file keeps updating. Change it to the new name when convenient.
- **Plex.** Signing in with Plex shows the name Yagi on plex.tv from now on. If you signed in before the rename, you are signed out of the admin pages once and need to sign in again.

The GitHub repository moved to [github.com/johnschult/yagi](https://github.com/johnschult/yagi); the old address redirects.

## Does the viewer need to install anything?

No. A viewer adds your tuner in Plex, Jellyfin or Emby by `host:port`, like any other HDHomeRun. See [Media servers](media-servers.md). They do need a network path to your machine; [Networking](networking.md) covers the options.

## Does it send anything over the internet?

Only to your tuners, plus, each switchable in Settings, SiliconDust's firmware changelog and program guide. Nothing about you or your viewers is sent. [Security](security.md) has the details.

## Is it free?

Yes. It is MIT licensed.

## Can I share my tuner with the public?

No, and Yagi isn't built for it. It is for sharing **your own antenna with your family and social circle**: the people in your life that you know and trust, on your own network, a VPN or Tailscale. It refuses connections from the public internet, and it is not meant for public or commercial use, or for sending a signal to people you don't know. Rules about receiving and passing on broadcast signals differ from place to place, and you are responsible for following the ones that apply to you.
