# Service Container Monitor (Agent)

โปรเจคนี้คือ Agent ขนาดเล็กสำหรับตรวจสอบสถานะ (Monitor) ของ Docker Containers โดยจะรายงานสถานะการทำงาน, การใช้ทรัพยากรเครื่อง (Resource Usage), และสุขภาพของระบบ (Health Checks) ผ่านทาง REST API พร้อมรองรับการแจ้งเตือนแบบ Real-time เมื่อสถานะมีการเปลี่ยนแปลง

## คุณสมบัติ (Features)
- **Docker Monitoring**: ค้นหาและตรวจสอบ Containers อัตโนมัติผ่าน Docker Socket
- **Resource Usage**: ติดตามการใช้ CPU, Memory, Network I/O, และ Disk I/O
- **Health Checks**:
  - ตรวจสอบสถานะ Health ของ Docker (`healthy`, `unhealthy`)
  - ตรวจสอบ HTTP Health Check ภายนอก (กำหนดได้ใน Config)
- **Log Monitoring**:
  - ตรวจจับ Error Keywords ใน Container Logs เช่น `ERROR`, `FATAL`, `panic`
  - แจ้งเตือนพร้อมระบุประเภท Error (เช่น `LOG_PANIC`, `LOG_FATAL`)
- **Real-time Alerting**:
  - ตรวจจับการเปลี่ยนแปลงสถานะ (เช่น Running -> Stopped)
  - แจ้งเตือนเข้า MQTT Broker อัตโนมัติ (`/from-HMS`)
  - **Standalone Telegram Alerts**: แจ้งเตือนเข้า Telegram ตรงๆ โดยไม่ต้องพึ่ง Service อื่น (ตั้งค่าผ่าน `.env`)
  - **Alert Resolution**: แจ้งเตือนเมื่อปัญหากลับมาเป็นปกติ (Resolved)
- **REST API**: ให้ข้อมูล Metrics และ Alerts ผ่าน JSON API
- **Security**:
  - Label Filtering (Whitelist) เพื่อป้องกันข้อมูลความลับหลุด
  - Environment Variable Support สำหรับ URLs ที่มี Secrets
- **Performance**:
  - Parallel Stats Collection (เร็วขึ้น 5-10x)
  - Concurrent HTTP Health Checks (ไม่บล็อก Main Loop)
  - Cache สำหรับข้อมูลที่ไม่เปลี่ยนแปลง (ลด API calls 30-50%)
- **Observability**: Structured JSON Logging สำหรับ ELK/Grafana

## สิ่งที่ต้องมี (Prerequisites)
- **Docker** และ **Docker Compose**
- **Go 1.22+** (เฉพาะกรณีต้องการรันแบบ Local Development)

## การตั้งค่า (Configuration)
Agent จะถูกตั้งค่าผ่านไฟล์ `config.json` โดยคุณสามารถจับคู่ Container แต่ละตัวกับ URL สำหรับ Health Check ได้

**ตัวอย่าง `config.json`**:
```json
{
  "poll_interval_ms": 2000,
  "stats_interval_ms": 5000,
  "http_timeout_ms": 2000,
  "containers": [
    {
      "container_name": "/my-service",
      "healthcheck_url": "http://my-service:8080/health",
      "log_keywords": ["ERROR", "panic", "FATAL"]
    },
    {
      "container_name": "/db-postgres",
      "healthcheck_url": "" 
    }
  ]
}
```
*หมายเหตุ: หาก `healthcheck_url` เป็นค่าว่าง Agent จะยึดสถานะตาม Docker Container State เท่านั้น*

### การใช้ Environment Variables (แนะนำ)
เพื่อความปลอดภัย คุณสามารถใช้ Environment Variables แทนการ Hardcode URLs ใน `config.json`:

1. **คัดลอกไฟล์ตัวอย่าง**:
   ```bash
   cp .env.example .env
   ```

2. **แก้ไขค่าใน `.env`**:
   ```bash
   RABBITMQ_URL=http://nest_rabbitmq:15672
   STATUS_MANAGER_URL=http://status-manager-service:80
   TELEGRAM_BOT_TOKEN=8328957963:... # สำหรับส่งแจ้งเตือน Telegram
   TELEGRAM_CHAT_ID=7080687040       # สำหรับส่งแจ้งเตือน Telegram
   ```

3. **ใช้ตัวแปรใน `config.json`**:
   ```json
   {
     "containers": [
       {
         "container_name": "/nest_rabbitmq",
         "healthcheck_url": "${RABBITMQ_URL}"
       }
     ]
   }
   ```

*หมายเหตุ: ไฟล์ `.env` จะไม่ถูก Commit เข้า Git เพื่อป้องกันการรั่วไหลของข้อมูลความลับ*

## การติดตั้งและใช้งาน (Installation & Running)

### วิธีที่ 1: รันด้วย Docker (แนะนำ)
วิธีนี้เหมาะสำหรับรันเป็น Service หรือเป็นส่วนหนึ่งของ Stack

1. **Build และ Run**:
   ```bash
   docker compose up --build -d
   ```
   
2. **ดู Logs**:
   ```bash
   docker compose logs -f container-monitor
   ```

### วิธีที่ 2: รันแบบ Local (สำหรับนักพัฒนา)
สำหรับผู้ที่ต้องการรันและแก้ไขโค้ดในเครื่อง (โดยไม่ต้อง build image ใหม่ทุกครั้ง):

1. **Build Binary**:
   ```bash
   go build -o agent cmd/agent/main.go
   ```

2. **Run Agent**:
   ```bash
   # ต้องมี Docker Desktop รันอยู่ และ config.json ใน directory เดียวกัน
   ./agent -config config.json
   ```

   หรือรันผ่าน `go run` โดยตรง:
   ```bash
   go run cmd/agent/main.go -config config.json
   ```

## รายละเอียด API (API Endpoints)
Agent จะเปิด HTTP Port `8080` (ค่าเริ่มต้น) ดังนี้:

### 1. ตรวจสอบสถานะ Agent (Health Check)
เช็คว่าตัว Agent เองทำงานอยู่หรือไม่
- **GET** `/health`
- **Response**:
  ```json
  {
    "ok": true,
    "version": "1.0.0",
    "last_poll_ts": 1700000000
  }
  ```

### 2. ดึงข้อมูล Container Metrics
ดึงข้อมูลสถานะและ Resource Usage ปัจจุบันของทุก Container ที่ตรวจพบ
- **GET** `/v1/containers`
- **Response**:
  ```json
  {
    "generated_at": 1700000000,
    "items": [
        {
            "container_name": "/my-service",
            "status": "running",
            "resources": { "cpu_percent": 0.5, "mem_percent": 12.0 ... },
            "main_status": "OK"
        }
    ]
  }
  ```

### 3. ดึงรายการแจ้งเตือน (Active Alerts)
ดึงรายการแจ้งเตือนที่ยังค้างอยู่ (เช่น Container หยุดทำงาน, HTTP Check ล้มเหลว)
- **GET** `/v1/alerts`
- **Response**:
  ```json
  {
    "items": [
        {
            "level": "error",
            "key": "my-service",
            "message": "Container my-service is STOPPED",
            "timestamp": 1700000000
        }
    ]
  }
  ```

## การแก้ปัญหาเบื้องต้น (Troubleshooting)
- **Permission Denied (Docker Socket)**:
  - ตรวจสอบว่า User ที่รัน Agent อยู่ในกลุ่ม `docker` หรือไม่
  - หรือทดลองรันด้วย `sudo`
- **ไม่เจอ Containers (Containers Not Found)**:
  - ตรวจสอบชื่อใน `config.json` ปกติชื่อ Docker มักขึ้นต้นด้วย `/` (เช่น `/my-service`)
  - Agent จะทำการเทียบชื่อโดยตัด `/` ข้างหน้าออกให้
