import yaml
import os
from typing import List, Optional
from pydantic import BaseModel, Field, ValidationError
from loguru import logger

class SiteConfig(BaseModel):
    url: str
    filename: str
    enabled: bool = True

class GitConfig(BaseModel):
    branch: str = "main"
    push_interval: int = 600

class WorkerConfig(BaseModel):
    id: str = "worker-default"

class AppConfig(BaseModel):
    worker: WorkerConfig = Field(default_factory=WorkerConfig)
    git: GitConfig = Field(default_factory=GitConfig)
    sites: List[SiteConfig] = Field(default_factory=list)

def load_yaml_config(path: str = "config.yaml") -> AppConfig:
    """Loads and validates the YAML configuration file."""
    if not os.path.exists(path):
        logger.warning(f"YAML config file not found at {path}. Using default empty config.")
        return AppConfig()

    try:
        with open(path, "r") as f:
            data = yaml.safe_load(f) or {}
        
        config = AppConfig(**data)
        logger.info(f"Loaded YAML config from {path} with {len(config.sites)} sites.")
        return config
    except ValidationError as e:
        logger.error(f"Invalid YAML configuration in {path}: {e}")
        raise
    except Exception as e:
        logger.error(f"Failed to load YAML config {path}: {e}")
        raise
