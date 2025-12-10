import base64
import binascii
import json
import sys
from typing import Any, Dict, Optional
from urllib.parse import urlparse, parse_qs

class ProxyParser:
    """
    A class to parse various proxy protocol URIs into a structured dictionary.
    """
    def parse(self, uri: str) -> Optional[Dict[str, Any]]:
        """
        Detects the protocol and parses the given URI.
        """
        if uri.startswith("vless://"):
            return self._parse_vless_uri(uri)
        elif uri.startswith("vmess://"):
            return self._parse_vmess_uri(uri)
        elif uri.startswith("trojan://"):
            return self._parse_trojan_uri(uri)
        elif uri.startswith("ss://"):
            return self._parse_ss_uri(uri)
        elif uri.startswith("hy2://"):
            return self._parse_hy2_uri(uri)
        else:
            print(f"Unsupported URI scheme: {uri}", file=sys.stderr)
            return None

    @staticmethod
    def _parse_vless_uri(uri: str) -> Optional[Dict[str, Any]]:
        """Parses a VLESS URI into a structured dictionary."""
        try:
            parsed = urlparse(uri)
            if not all([parsed.scheme == 'vless', parsed.username, parsed.hostname, parsed.port]):
                print(f"Skipping malformed VLESS URI: {uri}", file=sys.stderr)
                return None

            query_params = parse_qs(parsed.query)
            return {
                "protocol": "vless",
                "remark": parsed.fragment or "",
                "address": parsed.hostname,
                "port": parsed.port,
                "vless_id": parsed.username,
                "encryption": query_params.get("encryption", ["none"])[0],
                "security": query_params.get("security", ["none"])[0],
                "type": query_params.get("type", ["tcp"])[0],
                "host": query_params.get("host", [None])[0],
                "path": query_params.get("path", [None])[0],
                "sni": query_params.get("sni", [None])[0],
                "flow": query_params.get("flow", [None])[0],
                "fp": query_params.get("fp", [None])[0],
                "pbk": query_params.get("pbk", [None])[0],
                "sid": query_params.get("sid", [None])[0],
                "raw_uri": uri,
            }
        except (ValueError, AttributeError) as e:
            print(f"Error parsing VLESS URI: {uri}. Error: {e}", file=sys.stderr)
            return None

    @staticmethod
    def _parse_vmess_uri(uri: str) -> Optional[Dict[str, Any]]:
        """Parses a VMess URI into a structured dictionary."""
        try:
            encoded_part = uri.replace("vmess://", "")
            encoded_part += "=" * (-len(encoded_part) % 4)
            decoded_json = base64.b64decode(encoded_part).decode("utf-8")
            vmess_data = json.loads(decoded_json)

            return {
                "protocol": "vmess",
                "remark": vmess_data.get("ps", ""),
                "address": vmess_data.get("add"),
                "port": int(vmess_data.get("port", 0)),
                "vmess_id": vmess_data.get("id"),
                "security": vmess_data.get("scy", "auto"),
                "type": vmess_data.get("net", "tcp"),
                "host": vmess_data.get("host", None),
                "path": vmess_data.get("path", None),
                "tls": vmess_data.get("tls", "none"),
                "sni": vmess_data.get("sni", None),
                "aid": vmess_data.get("aid", 0),
                "raw_uri": uri,
            }
        except (json.JSONDecodeError, binascii.Error, TypeError) as e:
            print(f"Error parsing VMess URI: {uri}. Error: {e}", file=sys.stderr)
            return None

    @staticmethod
    def _parse_trojan_uri(uri: str) -> Optional[Dict[str, Any]]:
        """Parses a Trojan URI into a structured dictionary."""
        try:
            parsed = urlparse(uri)
            if not all([parsed.scheme == 'trojan', parsed.username, parsed.hostname, parsed.port]):
                print(f"Skipping malformed Trojan URI: {uri}", file=sys.stderr)
                return None

            query_params = parse_qs(parsed.query)
            return {
                "protocol": "trojan",
                "remark": parsed.fragment or "",
                "address": parsed.hostname,
                "port": parsed.port,
                "password": parsed.username,
                "sni": query_params.get("sni", [query_params.get("peer", [None])[0]])[0],
                "security": query_params.get("security", ["tls"])[0],
                "type": query_params.get("type", ["tcp"])[0],
                "flow": query_params.get("flow", [None])[0],
                "path": query_params.get("path", [None])[0],
                "host": query_params.get("host", [None])[0],
                "raw_uri": uri,
            }
        except (ValueError, AttributeError) as e:
            print(f"Error parsing Trojan URI: {uri}. Error: {e}", file=sys.stderr)
            return None

    @staticmethod
    def _parse_ss_uri(uri: str) -> Optional[Dict[str, Any]]:
        """Parses a Shadowsocks (SS) URI into a structured dictionary."""
        try:
            parsed = urlparse(uri)
            
            user_info_part, address_part = parsed.netloc.split('@', 1)

            user_info_decoded = base64.urlsafe_b64decode(user_info_part + '===').decode('utf-8')
            
            method, password = user_info_decoded.split(':', 1)
            host, port_str = address_part.rsplit(':', 1)
            port = int(port_str)

            return {
                "protocol": "shadowsocks",
                "remark": parsed.fragment or "",
                "address": host,
                "port": port,
                "method": method,
                "password": password,
                "raw_uri": uri,
            }
        except (ValueError, IndexError, binascii.Error) as e:
            print(f"Error parsing Shadowsocks URI: {uri}. Error: {e}", file=sys.stderr)
            return None

    @staticmethod
    def _parse_hy2_uri(uri: str) -> Optional[Dict[str, Any]]:
        """Parses a Hysteria 2 URI into a structured dictionary."""
        try:
            parsed = urlparse(uri)
            if parsed.scheme != 'hy2':
                return None

            # For hy2, the username part is used as the password/auth
            auth = parsed.username
            host = parsed.hostname
            port = parsed.port

            if not all([host, port, auth]):
                print(f"Skipping malformed Hysteria 2 URI: {uri}", file=sys.stderr)
                return None

            query_params = parse_qs(parsed.query)
            
            return {
                "protocol": "hysteria2",
                "remark": parsed.fragment or "",
                "address": host,
                "port": port,
                "password": auth,
                "sni": query_params.get("sni", [None])[0],
                "insecure": query_params.get("insecure", ["0"])[0] == "1",
                "obfs": query_params.get("obfs", [None])[0],
                "obfs_password": query_params.get("obfs-password", [None])[0],
                "raw_uri": uri,
            }
        except (ValueError, AttributeError) as e:
            print(f"Error parsing Hysteria 2 URI: {uri}. Error: {e}", file=sys.stderr)
            return None
