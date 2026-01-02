import os
import sys
import aiohttp
import geoip2.database
from typing import Optional, Tuple

class GeoIPService:
    def __init__(self, db_path: str = "Country.mmdb"):
        self.db_path = db_path
        self.reader: Optional[geoip2.database.Reader] = None
        self.download_url = "https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-Country.mmdb"

    async def initialize(self):
        """Downloads the DB if missing and opens the reader."""
        if not os.path.exists(self.db_path):
            print(f"GeoIP database not found at {self.db_path}. Downloading...")
            await self._download_db()
        
        try:
            self.reader = geoip2.database.Reader(self.db_path)
            print(f"GeoIP database loaded from {self.db_path}")
        except Exception as e:
            print(f"Failed to load GeoIP database: {e}", file=sys.stderr)

    async def _download_db(self):
        try:
            async with aiohttp.ClientSession() as session:
                async with session.get(self.download_url) as resp:
                    resp.raise_for_status()
                    with open(self.db_path, 'wb') as f:
                        while True:
                            chunk = await resp.content.read(1024*1024)
                            if not chunk:
                                break
                            f.write(chunk)
            print("GeoIP database download complete.")
        except Exception as e:
            print(f"Error downloading GeoIP database: {e}", file=sys.stderr)

    def get_country(self, ip: str) -> Tuple[str, str]:
        """Returns (country_code, flag_emoji). Defaults to ('UN', '🇺🇳')."""
        if not self.reader:
            return "UN", "🇺🇳"
            
        try:
            response = self.reader.country(ip)
            iso_code = response.country.iso_code
            if not iso_code:
                return "UN", "🇺🇳"
            
            return iso_code, self._get_flag_emoji(iso_code)
        except Exception:
            # IP not found or invalid
            return "UN", "🇺🇳"

    def _get_flag_emoji(self, country_code: str) -> str:
        """Converts a 2-letter country code to a flag emoji."""
        return "".join([chr(ord(c) + 127397) for c in country_code.upper()])

    def close(self):
        if self.reader:
            self.reader.close()
