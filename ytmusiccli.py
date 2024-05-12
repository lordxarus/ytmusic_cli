from ytmusicapi import YTMusic
import sys

ytmusic = YTMusic("oauth.json", "105811005689198404274")

# res = ytmusic.search(sys.argv[1], filter="songs")

import json

# https://stackoverflow.com/questions/36021332/how-to-prettyprint-human-readably-print-a-python-dict-in-json-format-double-q

ytmusic.add

home = YTMusic.search(ytmusic)
print(json.dumps(
    home,
    sort_keys=True,
    indent=4,
    separators=(',', ': ')
))