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

class WorkerConfig(BaseModel):
    id: str = Field(default="worker-01")

class GitConfig(BaseModel):
    branch: str = Field(default="main")
    push_interval: int = Field(default=600)

class AppYamlConfig(BaseModel):
    worker: WorkerConfig = Field(default_factory=WorkerConfig)
    git: GitConfig = Field(default_factory=GitConfig)
    sites: List[SiteConfig] = Field(default_factory=list)

class Settings(BaseSettings):
    # Xray Configuration
    XRAY_PATH: str = Field(default="/usr/local/bin/xray")
    XRAY_ASSETS_PATH: str = Field(default="/usr/local/bin/")

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
    BATCH_SIZE: int = Field(default=100)
    BASE_PORT: int = Field(default=20000)
    TEST_TIMEOUT: int = Field(default=10)
    MAX_DELAY_MS: int = Field(default=8000)
    MAX_CONCURRENT_BATCHES: int = Field(default=10)
    
    # Caching
    CACHE_INTERVAL_SECONDS: int = Field(default=900)
    SITE_CACHE_TTL_SECONDS: int = Field(default=86400)
    SITE_REQUEST_MAX_AGE_DAYS: int = Field(default=5)
    MAX_CONCURRENT_SITE_CHECKS: int = Field(default=3)
    MAX_FAIL_COUNT: int = Field(default=3)
    
    # Redis
    REDIS_HOST: str = Field(default="localhost")
    REDIS_PORT: int = Field(default=6379)
    REDIS_DB: int = Field(default=0)
    REDIS_PASSWORD: str = Field(default="")

    # GeoIP
    GEOIP_DB_PATH: str = Field(default="Country.mmdb")
    STATE_FILE_PATH: str = Field(default="state.json")

    # Server Configuration
    UVICORN_HOST: str = Field(default="0.0.0.0")
    UVICORN_PORT: int = Field(default=8084)

    # GitHub Integration
    GITHUB_PUSH_ENABLED: bool = Field(default=False)
    GITHUB_MAIN_PUSH_ENABLED: bool = Field(default=True)
    GITHUB_SITE_PUSH_ENABLED: bool = Field(default=True)
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
        if os.path.exists(path):
            try:
                with open(path, "r") as f:
                    data = yaml.safe_load(f) or {}
                
                self.app_config = AppYamlConfig(**data)
                
                # Override settings with YAML values if they are explicitly provided
                if "git" in data and isinstance(data["git"], dict):
                    if "branch" in data["git"]:
                        self.GITHUB_BRANCH = self.app_config.git.branch
                        logger.info(f"Overriding GITHUB_BRANCH from YAML: {self.GITHUB_BRANCH}")
                    if "push_interval" in data["git"]:
                        self.CACHE_INTERVAL_SECONDS = self.app_config.git.push_interval
                        logger.info(f"Overriding CACHE_INTERVAL_SECONDS from YAML: {self.CACHE_INTERVAL_SECONDS}")

                logger.info(f"Loaded YAML config from {path} with {len(self.app_config.sites)} sites.")
            except ValidationError as e:
                logger.error(f"Invalid YAML configuration in {path}: {e}")
            except Exception as e:
                logger.error(f"Failed to load YAML config {path}: {e}")
        else:
            logger.warning(f"YAML config file not found at {path}. Using default empty config.")

        # Fallback to PRECHECK_SITES if no sites configured in YAML
        if not self.app_config.sites and self.PRECHECK_SITES:
            fallback_sites = []
            urls = [self.PRECHECK_SITES] if isinstance(self.PRECHECK_SITES, str) else self.PRECHECK_SITES
            for url in urls:
                clean_host = url.replace("https://", "").replace("http://", "").replace("www.", "").split("/")[0]
                filename = f"{clean_host.replace('.', '_')}.txt"
                fallback_sites.append(SiteConfig(url=url, filename=filename, enabled=True))
            self.app_config.sites = fallback_sites
            logger.info(f"Populated fallback sites from PRECHECK_SITES: {len(fallback_sites)} sites.")

    model_config = SettingsConfigDict(env_file=".env", env_file_encoding="utf-8", extra="ignore")

settings = Settings()
settings.load_app_config()
