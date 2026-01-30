import sys
from loguru import logger

# Remove default handler
logger.remove()

# Add a new handler with the desired format
logger.add(
    sys.stderr,
    format="{time:YYYY-MM-DD HH:mm:ss} | {level: <8} | {message}",
    level="INFO",
)

# Export the configured logger
__all__ = ["logger"]
