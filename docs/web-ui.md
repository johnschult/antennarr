# A tour of the web page

Open it at `http://<antennarr address>:5004/antennarr/` (see [Getting started](getting-started.md#3-open-the-web-page)).
It has light and dark themes, and follows your computer's unless you choose one with the buttons at the top right.
These pictures are of a demo with a made-up tuner, guide and viewers.

## Live

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="screenshots/live-dark.jpg">
    <img src="screenshots/live-light.jpg" alt="The Live page: four tuners, one watched by a viewer and one by a local device, each with its channel, logo, what's on and how far into it, bitrate and signal, and a list of viewers" width="900">
  </picture>
</p>

What is happening right now. Each tuner has a header (its name, its model, how many of its tuners are live and its address), and one tile for each of its tuners:

- **Who has it.** A pill at the top: a viewer by name (globe), one of your own devices (house, shown as *Local*), a tuner held for your TVs (crown), or free.
- **What is on.** The channel, its logo, and, from SiliconDust's program guide, the show's picture faded behind the top of the tile, its title, category, episode and description, and how far into it you are. A tile without a picture is normal when the guide has none.
- **How it is doing.** The bitrate and the signal quality over the last couple of minutes, and the signal strength as bars (more bars, stronger) with where the stream is going. They are only colored when reception is fair (amber) or poor (red).

Below the tiles are your viewers, who is watching, and a timeline of events (a stream starting, a viewer turned away, a tuner offline). A tuner that is being restarted or updated is shown that way, not as offline. Turn the guide off in *Settings → Privacy and updates* and the tiles show only the channel.

## Viewers

<img src="screenshots/viewers.jpg" alt="The Viewers page: who can use the tuner, from which addresses, and how many tuners each gets" width="900">

Who may use the tuner, from which addresses, and how many tuners each gets at once. **Add viewer** walks through three steps (who, how their server reaches yours, and what to do in their media server); **Connection steps** on a row reopens it. See [Getting started](getting-started.md#4-add-a-viewer).

## Tuners

<img src="screenshots/tuners.jpg" alt="The Tuners page: a tuner's strip of what is on each of its tuners, with Manage, Device and Firmware sections" width="900">

One row for each tuner, opening to a strip of what is on each of its tuners and three sections:

- **Manage:** the name shown on this page (media servers are always told the device's own name), the port media servers use, and removing the tuner from Antennarr (the tuner itself is untouched).
- **Device:** what the tuner says about itself: hardware model, firmware, ATSC 3.0 DRM version, device ID, MAC address, IP address, subnet mask, channel maps and modulations. It is read when you open it and on **Refresh**, and it is read-only. **Restart** turns the tuner off and on again, after asking, and says if anything is in use.
- **Firmware:** the version, when SiliconDust's changelog was last read, and what changed. When a newer release applies to your tuner there is an **Install** button; see [Configuration](configuration.md) for what it does and [Security](security.md) for what it contacts.

## Activity

<img src="screenshots/activity.jpg" alt="The Activity page: time watched, the most watched channel, who watched most, a bar for each day and the sessions by day" width="900">

Who watched what, and for how long, for the last 7 or 30 days: the total time, how much data went out to viewers (about, through Antennarr, which is what your upload paid for), the most watched channel, who watched most, the busiest day, a bar for each day, a list by viewer (time and data) and by channel (time and the average signal quality, colored when it is fair or poor), and the sessions by day with the shows that were on. A viewer's name opens their own page: their data and time for each day, what they watched and their sessions. It is kept for 30 days in a file next to the settings file and goes nowhere else. Switch it off, or clear it, in *Settings → Privacy and updates*; the *Activity* item then leaves the top bar. See [Configuration](configuration.md#files-antennarr-keeps).

## Settings

<img src="screenshots/settings.jpg" alt="The Settings page, at its privacy and updates section" width="900">

Who may open this page, the limits, notifications (ntfy, Pushover or a webhook), Sign in with Plex, and the switches for what Antennarr reads and keeps: the firmware check, the program guide and the activity history. Every setting is explained in [Configuration](configuration.md).

## On a phone

<p>
  <img src="screenshots/phone-live.jpg" alt="The Live page on a phone" width="220">
  &nbsp;
  <img src="screenshots/phone-menu.jpg" alt="The navigation menu on a phone" width="220">
</p>

The same pages, with a menu and cards instead of wide tables.
