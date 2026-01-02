from typing import List, Optional
from pydantic import BaseModel 


# --- Pydantic Models for API Response ---
class Server(BaseModel):
    protocol: str
    remark: str
    address: str
    port: int
    vless_id: Optional[str] = None
    vmess_id: Optional[str] = None
    password: Optional[str] = None
    raw_uri: str
    delay: int
    country_code: str = "UN"
    flag: str = "🇺🇳"

class ServerResponse(BaseModel):
    count: int
    servers: List[Server]
