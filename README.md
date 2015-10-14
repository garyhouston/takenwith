This is a bot for adding camera and scanner categories to image files
on [Wikimedia
Commons](https://commons.wikimedia.org/wiki/Main_Page). It's run by
[User:BotAdventures](https://commons.wikimedia.org/wiki/User:BotAdventures).

It's written in Go and uses
[go-mwclient](https://github.com/cgt/go-mwclient) to interface with
the [MediaWiki action
API](https://www.mediawiki.org/wiki/API:Main_page).

The code is nothing special, most of the work is found in the mapping
file from Exif fields to Commons categories (catmapping). The first
two fields are the device manufacturer and model from Exif. The third
field is the Commons category, where "Category:Taken with " is to be
prepended in most cases. Since I doubt that anybody else will want to
run this bot, I haven't included much more in the way of
documentation.

This package is freely licensed using the MIT License, see file
LICENSE for details. I also release the catmapping file to the
 public domain (in case it's even copyrightable) using
(Creative Commons Zero)[https://creativecommons.org/publicdomain/zero/1.0/].
