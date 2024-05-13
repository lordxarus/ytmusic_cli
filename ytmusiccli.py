from ytmusicapi import YTMusic
import json
import sys
import os
from dotenv import load_dotenv

def pp(obj):
 return (json.dumps(
	  	obj,
	  	sort_keys=True,
	  	indent=4,
	  	separators=(',', ': ')))


# Load the .env file
load_dotenv()

# Now you can access the variables as environment variables
brandId = os.getenv('BRAND_ID')
oauth = os.getenv('OAUTH_TOKEN')
videoId = "e3v60CXHnTA"

ytmusic = YTMusic(oauth, brandId)

res = ytmusic.get_song(videoId, ytmusic.get_signatureTimestamp())
			
# https://stackoverflow.com/questions/36021332/how-to-prettyprint-human-readably-print-a-python-dict-in-json-format-double-q
# print(json.dumps(
#  	res,
#  	sort_keys=True,
#  	indent=4,
#  	separators=(',', ': ')
#  ))

# json.loads(json.dumps(res))
# print(pp(ytmusic.rate_song(videoId, "LIKE")))
# print(ytmusic.add_history_item(res))
# song = ytmusic.get_song(videoId, ytmusic.get_signatureTimestamp())
print(pp(ytmusic.add_history_item()))