
# step 1 
Do this to install the xray in your ubuntu or debian. 
```
sudo bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)"
```
# step 2 
```
python -m venv .venv 
source .venv/bin/activate 
pip install -r requirement.txt 
```


# 3 making it a service : 


! use: 
```
which docker compose
```

```
[Unit]
Description=My Docker Compose Service
Requires=docker.service
After=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/path/to/your/docker-compose-project
ExecStart=/usr/local/bin/docker compose up -d
ExecStop=/usr/local/bin/docker compose down
TimeoutStartSec=0

[Install]
WantedBy=multi-user.target

```

