# Start from a Python 3.11 base image
FROM python:3.11-slim

# Set the working directory inside the container
WORKDIR /app

# Install necessary packages and download the Xray binary directly
RUN apt-get update && apt-get install -y wget unzip git \
    && wget -O /tmp/Xray-linux-64.zip https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-64.zip \
    && unzip /tmp/Xray-linux-64.zip -d /usr/local/bin/ \
    && chmod +x /usr/local/bin/xray \
    && rm /tmp/Xray-linux-64.zip

# Copy the requirements file and install Python packages
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy your application source code
COPY ./src .

# Expose the port the app runs on
EXPOSE 8084

# Command to run the application
# Note: Uvicorn is run directly, not from the __main__ block in your script
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8084"]
