# Milestone 1 Web Based Development
To run the project
```bash
docker compose up -d --build
```

Test if the service is working properly
```bash
curl http://localhost:28080/health
```

Should responded with
```bash
{
    "status":"ok"
}
```