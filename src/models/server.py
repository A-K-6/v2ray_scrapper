from typing import List, Optional, Dict, Any, Union
from pydantic import BaseModel, Field, computed_field
import base64
import json
import hashlib
from urllib.parse import quote, urlencode

class BaseProxyServer(BaseModel):
    protocol: str
    remark: str = ""
    address: str
    port: int
    delay: int = 0
    country_code: str = "UN"
    flag: str = "🇺🇳"
    fail_count: int = 0
    raw_uri: str = ""

    @computed_field
    @property
    def fingerprint(self) -> str:
        """Generates a unique hash for a server based on its connection details."""
        # Use only the base fields defined in the model, excluding transient state
        # self.__class__.model_fields.keys() gives all defined fields (including those in subclasses)
        data = {k: getattr(self, k) for k in self.__class__.model_fields.keys() if k not in {"remark", "delay", "country_code", "flag", "fail_count", "raw_uri"}}
        
        # Sort keys to ensure consistent hashing
        serialized = json.dumps(data, sort_keys=True)
        return hashlib.sha256(serialized.encode()).hexdigest()

    def to_uri(self) -> str:
        raise NotImplementedError

    def to_xray_outbound(self) -> Dict[str, Any]:
        raise NotImplementedError

class VlessServer(BaseProxyServer):
    protocol: str = "vless"
    vless_id: str
    encryption: str = "none"
    security: str = "none"
    type: str = "tcp"
    host: Optional[str] = None
    path: Optional[str] = None
    sni: Optional[str] = None
    flow: Optional[str] = None
    fp: Optional[str] = None
    pbk: Optional[str] = None
    sid: Optional[str] = None

    def to_uri(self) -> str:
        params = {
            "encryption": self.encryption,
            "security": self.security,
            "type": self.type,
            "host": self.host,
            "path": self.path,
            "sni": self.sni,
            "flow": self.flow,
            "fp": self.fp,
            "pbk": self.pbk,
            "sid": self.sid,
        }
        params = {k: v for k, v in params.items() if v is not None}
        query = urlencode(params)
        remark_part = f"#{quote(self.remark)}" if self.remark else ""
        return f"vless://{self.vless_id}@{self.address}:{self.port}?{query}{remark_part}"

    def to_xray_outbound(self) -> Dict[str, Any]:
        if self.type in ("http", "h2"):
            return None
        vnext = [{"address": self.address, "port": self.port, "users": [{
            "id": self.vless_id, "encryption": "none", "flow": self.flow or ""
        }]}]
        stream_settings = {"network": self.type, "security": self.security}
        if stream_settings["security"] == "auto":
            stream_settings["security"] = "none"

        if stream_settings["network"] == "ws":
            stream_settings["wsSettings"] = {"path": self.path or "/"}
            if self.host:
                stream_settings["wsSettings"]["host"] = self.host
        
        if stream_settings["security"] in ("tls", "reality"):
            security_settings = {"serverName": self.sni or self.host or self.address, "fingerprint": self.fp or "chrome"}
            if stream_settings["security"] == "reality":
                security_settings.update({"publicKey": self.pbk, "shortId": self.sid})
            setting_key = f"{stream_settings['security']}Settings"
            stream_settings[setting_key] = security_settings
        
        return {"protocol": "vless", "settings": {"vnext": vnext}, "streamSettings": stream_settings}

class VmessServer(BaseProxyServer):
    protocol: str = "vmess"
    vmess_id: str
    security: str = "auto"
    type: str = "tcp"
    host: Optional[str] = None
    path: Optional[str] = None
    tls: str = "none"
    sni: Optional[str] = None
    aid: int = 0

    def to_uri(self) -> str:
        data = {
            "v": "2",
            "ps": self.remark,
            "add": self.address,
            "port": str(self.port),
            "id": self.vmess_id,
            "aid": self.aid,
            "scy": self.security,
            "net": self.type,
            "type": "none",
            "host": self.host or "",
            "path": self.path or "",
            "tls": self.tls,
            "sni": self.sni or ""
        }
        json_str = json.dumps({k: v for k, v in data.items() if v is not None}, separators=(',', ':'))
        b64_encoded = base64.b64encode(json_str.encode()).decode()
        return f"vmess://{b64_encoded}"

    def to_xray_outbound(self) -> Dict[str, Any]:
        if self.type in ("http", "h2"):
            return None
        vnext = [{"address": self.address, "port": self.port, "users": [{
            "id": self.vmess_id, "alterId": self.aid, "security": self.security
        }]}]
        stream_settings = {"network": self.type, "security": self.tls}
        if stream_settings["security"] == "auto":
            stream_settings["security"] = "none"

        if stream_settings["network"] == "ws":
            stream_settings["wsSettings"] = {"path": self.path or "/"}
            if self.host:
                stream_settings["wsSettings"]["host"] = self.host
        
        if stream_settings["security"] == "tls":
            stream_settings["tlsSettings"] = {"serverName": self.sni or self.host or self.address}
        
        return {"protocol": "vmess", "settings": {"vnext": vnext}, "streamSettings": stream_settings}

class TrojanServer(BaseProxyServer):
    protocol: str = "trojan"
    password: str
    sni: Optional[str] = None
    security: str = "tls"
    type: str = "tcp"
    flow: Optional[str] = None
    path: Optional[str] = None
    host: Optional[str] = None

    def to_uri(self) -> str:
        params = {
            "security": self.security,
            "sni": self.sni,
            "type": self.type,
            "flow": self.flow,
            "path": self.path,
            "host": self.host,
        }
        params = {k: v for k, v in params.items() if v is not None}
        query = urlencode(params)
        remark_part = f"#{quote(self.remark)}" if self.remark else ""
        return f"trojan://{self.password}@{self.address}:{self.port}?{query}{remark_part}"

    def to_xray_outbound(self) -> Dict[str, Any]:
        if self.type in ("http", "h2"):
            return None
        server_config = [{"address": self.address, "port": self.port, "password": self.password}]
        stream_settings = {"network": self.type, "security": "tls"}
        stream_settings["tlsSettings"] = {"serverName": self.sni or self.host or self.address}
        if self.type == "ws":
            stream_settings["wsSettings"] = {"path": self.path or "/"}
            if self.host:
                stream_settings["wsSettings"]["host"] = self.host
        return {"protocol": "trojan", "settings": {"servers": server_config}, "streamSettings": stream_settings}

class ShadowsocksServer(BaseProxyServer):
    protocol: str = "shadowsocks"
    method: str
    password: str

    def to_uri(self) -> str:
        user_info = f"{self.method}:{self.password}"
        user_info_b64 = base64.urlsafe_b64encode(user_info.encode()).decode().strip('=')
        remark_part = f"#{quote(self.remark)}" if self.remark else ""
        return f"ss://{user_info_b64}@{self.address}:{self.port}{remark_part}"

    def to_xray_outbound(self) -> Dict[str, Any]:
        # List of supported AEAD ciphers in modern Xray
        supported_methods = [
            "aes-128-gcm", "aes-256-gcm", "chacha20-ietf-poly1305", 
            "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm"
        ]
        if self.method not in supported_methods:
            return None

        server_config = [{
            "address": self.address, 
            "port": self.port, 
            "method": self.method, 
            "password": self.password
        }]
        return {"protocol": "shadowsocks", "settings": {"servers": server_config}}

class Hysteria2Server(BaseProxyServer):
    protocol: str = "hysteria2"
    password: str
    sni: Optional[str] = None
    insecure: bool = False
    obfs: Optional[str] = None
    obfs_password: Optional[str] = None

    def to_uri(self) -> str:
        params = {
            "sni": self.sni,
            "obfs": self.obfs,
            "obfs-password": self.obfs_password,
            "insecure": "1" if self.insecure else None,
        }
        params = {k: v for k, v in params.items() if v is not None}
        query = urlencode(params)
        remark_part = f"#{quote(self.remark)}" if self.remark else ""
        return f"hy2://{self.password}@{self.address}:{self.port}?{query}{remark_part}"

    def to_xray_outbound(self) -> Dict[str, Any]:
        server_info = {
            "address": self.address,
            "port": self.port,
            "password": self.password
        }
        if self.obfs and self.obfs != "none":
             server_info["obfs"] = {
                "type": self.obfs,
                "password": self.obfs_password or ""
            }
        
        stream_settings = {
            "security": "tls",
            "tlsSettings": {
                "serverName": self.sni or self.address,
                "allowInsecure": self.insecure
            }
        }
        return {
            "protocol": "hysteria2",
            "settings": {"servers": [server_info]},
            "streamSettings": stream_settings
        }

# --- Pydantic Models for API Response ---
ProxyServer = Union[VlessServer, VmessServer, TrojanServer, ShadowsocksServer, Hysteria2Server]

class ServerResponse(BaseModel):
    count: int
    servers: List[ProxyServer]
