import base64
import json
from urllib.parse import quote, urlencode

class UriGenerator:
    """
    Helper class to regenerate proxy URIs from server dictionaries.
    Useful for updating remarks/names.
    """

    @staticmethod
    def generate(server: dict) -> str:
        protocol = server.get("protocol")
        if protocol == "vless":
            return UriGenerator._generate_vless(server)
        elif protocol == "vmess":
            return UriGenerator._generate_vmess(server)
        elif protocol == "trojan":
            return UriGenerator._generate_trojan(server)
        elif protocol == "shadowsocks":
            return UriGenerator._generate_ss(server)
        elif protocol == "hysteria2":
            return UriGenerator._generate_hy2(server)
        else:
            return server.get("raw_uri", "")

    @staticmethod
    def _generate_vless(s: dict) -> str:
        # vless://uuid@host:port?params#remark
        user = s.get("vless_id", "")
        host = s.get("address", "")
        port = s.get("port", "")
        
        params = {}
        if s.get("encryption"): params["encryption"] = s["encryption"]
        if s.get("security"): params["security"] = s["security"]
        if s.get("type"): params["type"] = s["type"]
        if s.get("host"): params["host"] = s["host"]
        if s.get("path"): params["path"] = s["path"]
        if s.get("sni"): params["sni"] = s["sni"]
        if s.get("flow"): params["flow"] = s["flow"]
        if s.get("fp"): params["fp"] = s["fp"]
        if s.get("pbk"): params["pbk"] = s["pbk"]
        if s.get("sid"): params["sid"] = s["sid"]
        
        # Filter None values
        params = {k: v for k, v in params.items() if v is not None}
        
        query = urlencode(params)
        remark = quote(s.get("remark", ""))
        
        return f"vless://{user}@{host}:{port}?{query}#{remark}"

    @staticmethod
    def _generate_vmess(s: dict) -> str:
        # vmess://base64(json)
        data = {
            "v": "2",
            "ps": s.get("remark", ""),
            "add": s.get("address", ""),
            "port": str(s.get("port", "")),
            "id": s.get("vmess_id", ""),
            "aid": s.get("aid", 0),
            "scy": s.get("security", "auto"),
            "net": s.get("type", "tcp"),
            "type": "none", # Often static
            "host": s.get("host", ""),
            "path": s.get("path", ""),
            "tls": s.get("tls", ""),
            "sni": s.get("sni", "")
        }
        
        # Clean up keys that might be None
        data = {k: v for k, v in data.items() if v is not None}
        
        json_str = json.dumps(data, separators=(',', ':'))
        b64_encoded = base64.b64encode(json_str.encode()).decode()
        return f"vmess://{b64_encoded}"

    @staticmethod
    def _generate_trojan(s: dict) -> str:
        # trojan://password@host:port?params#remark
        password = s.get("password", "")
        host = s.get("address", "")
        port = s.get("port", "")
        
        params = {}
        if s.get("security"): params["security"] = s["security"]
        if s.get("sni"): params["sni"] = s["sni"]
        if s.get("type"): params["type"] = s["type"]
        if s.get("flow"): params["flow"] = s["flow"]
        if s.get("path"): params["path"] = s["path"]
        if s.get("host"): params["host"] = s["host"]
        
        params = {k: v for k, v in params.items() if v is not None}
        query = urlencode(params)
        remark = quote(s.get("remark", ""))
        
        return f"trojan://{password}@{host}:{port}?{query}#{remark}"

    @staticmethod
    def _generate_ss(s: dict) -> str:
        # ss://base64(method:password)@host:port#remark
        method = s.get("method", "")
        password = s.get("password", "")
        host = s.get("address", "")
        port = s.get("port", "")
        
        user_info = f"{method}:{password}"
        user_info_b64 = base64.urlsafe_b64encode(user_info.encode()).decode().strip('=')
        
        remark = quote(s.get("remark", ""))
        return f"ss://{user_info_b64}@{host}:{port}#{remark}"

    @staticmethod
    def _generate_hy2(s: dict) -> str:
        # hy2://auth@host:port?params#remark
        auth = s.get("password", "")
        host = s.get("address", "")
        port = s.get("port", "")
        
        params = {}
        if s.get("sni"): params["sni"] = s["sni"]
        if s.get("obfs"): params["obfs"] = s["obfs"]
        if s.get("obfs_password"): params["obfs-password"] = s["obfs_password"]
        if s.get("insecure"): params["insecure"] = "1"
        
        params = {k: v for k, v in params.items() if v is not None}
        query = urlencode(params)
        remark = quote(s.get("remark", ""))
        
        return f"hy2://{auth}@{host}:{port}?{query}#{remark}"
