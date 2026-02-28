import sys
import os
from loguru import logger

# Get log level from environment or default to INFO
log_level = os.getenv("LOG_LEVEL", "INFO").upper()

# Remove default handler
logger.remove()

# Add a new handler with the desired format and level
logger.add(
    sys.stderr,
    format="{time:YYYY-MM-DD HH:mm:ss} | {level: <8} | {message}",
    level=log_level,
)

# Export the configured logger
__all__ = ["logger"]
