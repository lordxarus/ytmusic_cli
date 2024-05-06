from ytmusicapi import YTMusic
import sys

ytmusic = YTMusic("oauth.json")

res = ytmusic.search(sys.argv[1], filter="songs")

import json

# https://stackoverflow.com/questions/36021332/how-to-prettyprint-human-readably-print-a-python-dict-in-json-format-double-q
print(json.dumps(
    res,
    sort_keys=True,
    indent=4,
    separators=(',', ': ')
))

