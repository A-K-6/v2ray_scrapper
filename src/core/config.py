import os
import json
from typing import List, Union, Any
from pydantic_settings import BaseSettings, SettingsConfigDict
from pydantic import Field, field_validator, AliasChoices

class Settings(BaseSettings):
    # Xray Configuration
    XRAY_PATH: str = Field(default="/usr/local/bin/xray")
    XRAY_ASSETS_PATH: str = Field(default="/usr/share/xray/")

    # Subscription Configuration
    # Fallback to SUB_URL if SUB_URLS is not set for backward compatibility
    _DEFAULT_SUB: str = "https://github.com/Epodonios/v2ray-configs/raw/main/Splitted-By-Protocol/vless.txt"
    
    SUB_URLS: Union[List[str], str] = Field(
        default=["https://github.com/Epodonios/v2ray-configs/raw/main/Splitted-By-Protocol/vless.txt"],
        validation_alias=AliasChoices("SUB_URLS", "SUB_URL")
    )

    # Features
    LOW_INTERNET_CONS: bool = Field(default=False)
    LOW_INTERNET_LIMIT: int = Field(default=50)
    
    PRECHECK_SITES: Union[List[str], str] = Field(default=[])

    @field_validator("SUB_URLS", "PRECHECK_SITES", mode="before")
    @classmethod
    def parse_comma_separated_list(cls, v: Any) -> List[str]:
        if isinstance(v, str):
            v = v.strip()
            # Try to parse as JSON first (in case user provided ["url1", "url2"])
            if v.startswith("[") and v.endswith("]"):
                try:
                    return json.loads(v)
                except json.JSONDecodeError:
                    # If JSON fails (e.g. malformed), fall back to splitting by comma
                    pass
            
            # Split by comma
            return [s.strip() for s in v.split(",") if s.strip()]
        return v

    # Testing Configuration
    LATENCY_TEST_URL: str = Field(default="http://www.google.com/generate_204")
    BATCH_SIZE: int = Field(default=500)
    BASE_PORT: int = Field(default=20000)
    TEST_TIMEOUT: int = Field(default=10)
    MAX_DELAY_MS: int = Field(default=8000)
    
    # Caching
    CACHE_INTERVAL_SECONDS: int = Field(default=900) # 15 minutes
    SITE_CACHE_TTL_SECONDS: int = Field(default=3600) # 1 hour
    
    # Redis
    REDIS_HOST: str = Field(default="localhost")
    REDIS_PORT: int = Field(default=6379)
    REDIS_DB: int = Field(default=0)
    REDIS_PASSWORD: str = Field(default="")

    # GeoIP
    GEOIP_DB_PATH: str = Field(default="Country.mmdb")

    # Server Configuration
    UVICORN_HOST: str = Field(default="0.0.0.0")
    UVICORN_PORT: int = Field(default=8084)

    # GitHub Integration
    GITHUB_PUSH_ENABLED: bool = Field(default=False)
    GITHUB_TOKEN: str = Field(default="")
    GITHUB_REPO_URL: str = Field(default="")
    GITHUB_USER: str = Field(default="V2Ray Updater")
    GITHUB_EMAIL: str = Field(default="bot@example.com")
    GITHUB_BRANCH: str = Field(default="main")
    GITHUB_FILENAME: str = Field(default="subscription.txt")
    GITHUB_REPO_DIR: str = Field(default="/app/subscription_repo")

    model_config = SettingsConfigDict(env_file=".env", env_file_encoding="utf-8", extra="ignore")

settings = Settings()
