import os
import json
import yaml
from typing import List, Union, Any, Optional
from pydantic_settings import BaseSettings, SettingsConfigDict
from pydantic import Field, field_validator, AliasChoices, BaseModel, ValidationError
from loguru import logger

class SiteConfig(BaseModel):
    url: str
    filename: str
    enabled: bool = True

class AppYamlConfig(BaseModel):
    sites: List[SiteConfig] = Field(default_factory=list)
    # Add other YAML-specific fields here if needed

class Settings(BaseSettings):
    # Xray Configuration
    XRAY_PATH: str = Field(default="/usr/local/bin/xray")
    XRAY_ASSETS_PATH: str = Field(default="/usr/share/xray/")

    # Subscription Configuration
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
            if v.startswith("[") and v.endswith("]"):
                try:
                    return json.loads(v)
                except json.JSONDecodeError:
                    pass
            return [s.strip() for s in v.split(",") if s.strip()]
        return v

    # Testing Configuration
    LATENCY_TEST_URL: str = Field(default="http://www.google.com/generate_204")
    BATCH_SIZE: int = Field(default=500)
    BASE_PORT: int = Field(default=20000)
    TEST_TIMEOUT: int = Field(default=10)
    MAX_DELAY_MS: int = Field(default=8000)
    
    # Caching
    CACHE_INTERVAL_SECONDS: int = Field(default=900)
    SITE_CACHE_TTL_SECONDS: int = Field(default=3600)
    MAX_FAIL_COUNT: int = Field(default=3)
    
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

    # Git Proxy Settings
    GIT_HTTP_PROXY: str = Field(default="")
    GIT_HTTPS_PROXY: str = Field(default="")
    GIT_SSL_NO_VERIFY: bool = Field(default=False)
    GIT_SELF_PROXY_ENABLED: bool = Field(default=True)

    # YAML Config Path
    YAML_CONFIG_PATH: str = Field(default="config.yaml")

    # Nested App Config (from YAML)
    app_config: AppYamlConfig = Field(default_factory=AppYamlConfig)

    def load_app_config(self):
        """Loads and validates the YAML configuration file."""
        path = self.YAML_CONFIG_PATH
        if not os.path.exists(path):
            logger.warning(f"YAML config file not found at {path}. Using default empty config.")
            return

        try:
            with open(path, "r") as f:
                data = yaml.safe_load(f) or {}
            
            self.app_config = AppYamlConfig(**data)
            logger.info(f"Loaded YAML config from {path} with {len(self.app_config.sites)} sites.")
        except ValidationError as e:
            logger.error(f"Invalid YAML configuration in {path}: {e}")
        except Exception as e:
            logger.error(f"Failed to load YAML config {path}: {e}")

    model_config = SettingsConfigDict(env_file=".env", env_file_encoding="utf-8", extra="ignore")

settings = Settings()
settings.load_app_config()
