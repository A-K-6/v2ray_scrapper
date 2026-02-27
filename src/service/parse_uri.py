import base64
import binascii
import json
import re
from typing import Optional, Union
from urllib.parse import urlparse, parse_qs
from loguru import logger

from models.server import (
    VlessServer, VmessServer, TrojanServer, 
    ShadowsocksServer, Hysteria2Server, ProxyServer
)

class ProxyParser:
    """
    A class to parse various proxy protocol URIs into specialized Pydantic models.
    """
    def parse(self, uri: str) -> Optional[ProxyServer]:
        """
        Detects the protocol and parses the given URI.
        """
        uri = uri.strip()
        uri = re.sub(r'\s+([#?])', r'\1', uri)
        if not uri:
            return None
        if uri.startswith("vless://"):
            return self._parse_vless_uri(uri)
        elif uri.startswith("vmess://"):
            return self._parse_vmess_uri(uri)
        elif uri.startswith("trojan://"):
            return self._parse_trojan_uri(uri)
        elif uri.startswith("ss://"):
            return self._parse_ss_uri(uri)
        elif uri.startswith("hy2://") or uri.startswith("hysteria2://"):
            return self._parse_hy2_uri(uri)
        return None

    @staticmethod
    def _parse_vless_uri(uri: str) -> Optional[VlessServer]:
        """Parses a VLESS URI into a VlessServer model."""
        try:
            parsed = urlparse(uri)
            if not all([parsed.scheme == 'vless', parsed.username, parsed.hostname, parsed.port]):
                return None

            query_params = parse_qs(parsed.query)
            return VlessServer(
                remark=parsed.fragment or "",
                address=parsed.hostname,
                port=parsed.port,
                vless_id=parsed.username,
                encryption=query_params.get("encryption", ["none"])[0],
                security=query_params.get("security", ["none"])[0],
                type=query_params.get("type", ["tcp"])[0],
                host=query_params.get("host", [None])[0],
                path=query_params.get("path", [None])[0],
                sni=query_params.get("sni", [None])[0],
                flow=query_params.get("flow", [None])[0],
                fp=query_params.get("fp", [None])[0],
                pbk=query_params.get("pbk", [None])[0],
                sid=query_params.get("sid", [None])[0],
                raw_uri=uri,
            )
        except Exception as e:
            logger.debug(f"Error parsing VLESS URI: {e}")
            return None

    @staticmethod
    def _parse_vmess_uri(uri: str) -> Optional[VmessServer]:
        """Parses a VMess URI into a VmessServer model."""
        try:
            encoded_part = uri.replace("vmess://", "")
            if "?" in encoded_part:
                encoded_part = encoded_part.split("?")[0]
            
            encoded_part = encoded_part.strip()
            padding_needed = 4 - (len(encoded_part) % 4)
            if padding_needed and padding_needed != 4:
                encoded_part += "=" * padding_needed
                
            decoded_bytes = base64.b64decode(encoded_part, validate=False)
            decoded_json = decoded_bytes.decode("utf-8", errors="ignore")
            
            last_brace_index = decoded_json.rfind("}")
            if last_brace_index != -1:
                decoded_json = decoded_json[:last_brace_index+1]
            
            vmess_data = json.loads(decoded_json)

            return VmessServer(
                remark=vmess_data.get("ps", ""),
                address=vmess_data.get("add"),
                port=int(vmess_data.get("port", 0)),
                vmess_id=vmess_data.get("id"),
                security=vmess_data.get("scy", "auto"),
                type=vmess_data.get("net", "tcp"),
                host=vmess_data.get("host"),
                path=vmess_data.get("path"),
                tls=vmess_data.get("tls", "none"),
                sni=vmess_data.get("sni"),
                aid=int(vmess_data.get("aid", 0)),
                raw_uri=uri,
            )
        except Exception as e:
            logger.debug(f"Error parsing VMess URI: {e}")
            return None

    @staticmethod
    def _parse_trojan_uri(uri: str) -> Optional[TrojanServer]:
        """Parses a Trojan URI into a TrojanServer model."""
        try:
            parsed = urlparse(uri)
            if not all([parsed.scheme == 'trojan', parsed.username, parsed.hostname, parsed.port]):
                return None

            query_params = parse_qs(parsed.query)
            return TrojanServer(
                remark=parsed.fragment or "",
                address=parsed.hostname,
                port=parsed.port,
                password=parsed.username,
                sni=query_params.get("sni", [query_params.get("peer", [None])[0]])[0],
                security=query_params.get("security", ["tls"])[0],
                type=query_params.get("type", ["tcp"])[0],
                flow=query_params.get("flow", [None])[0],
                path=query_params.get("path", [None])[0],
                host=query_params.get("host", [None])[0],
                raw_uri=uri,
            )
        except Exception as e:
            logger.debug(f"Error parsing Trojan URI: {e}")
            return None

    @staticmethod
    def _parse_ss_uri(uri: str) -> Optional[ShadowsocksServer]:
        """Parses a Shadowsocks (SS) URI into a ShadowsocksServer model."""
        try:
            parsed = urlparse(uri)
            user_info_part = parsed.netloc
            if '@' in user_info_part:
                user_info_part, address_part = user_info_part.split('@', 1)
            else:
                 return None

            padding = 4 - (len(user_info_part) % 4)
            if padding != 4:
                user_info_part += '=' * padding
            
            user_info_decoded = base64.urlsafe_b64decode(user_info_part).decode('utf-8')
            if ':' not in user_info_decoded:
                return None
                
            method, password = user_info_decoded.split(':', 1)
            if ':' in address_part:
                host, port_str = address_part.rsplit(':', 1)
                port = int(port_str)
            else:
                return None

            remark = ""
            if parsed.fragment:
                from urllib.parse import unquote
                remark = unquote(parsed.fragment)

            return ShadowsocksServer(
                remark=remark,
                address=host,
                port=port,
                method=method,
                password=password,
                raw_uri=uri,
            )
        except Exception as e:
            logger.debug(f"Error parsing Shadowsocks URI: {e}")
            return None

    @staticmethod
    def _parse_hy2_uri(uri: str) -> Optional[Hysteria2Server]:
        """Parses a Hysteria 2 URI into a Hysteria2Server model."""
        try:
            parsed = urlparse(uri)
            if parsed.scheme not in ('hy2', 'hysteria2'):
                return None

            auth = parsed.username
            host = parsed.hostname
            port = parsed.port
            if not all([host, port, auth]):
                return None

            query_params = parse_qs(parsed.query)
            return Hysteria2Server(
                remark=parsed.fragment or "",
                address=host,
                port=port,
                password=auth,
                sni=query_params.get("sni", [None])[0],
                insecure=query_params.get("insecure", ["0"])[0] == "1",
                obfs=query_params.get("obfs", [None])[0],
                obfs_password=query_params.get("obfs-password", [None])[0],
                raw_uri=uri,
            )
        except Exception as e:
            logger.debug(f"Error parsing Hysteria 2 URI: {e}")
            return None
