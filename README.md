# Deploy del backend de Bingo — Guía completa

Guía para desplegar cambios del backend (Go) a la VM de Oracle Cloud.
El backend corre como servicio systemd detrás de Caddy (HTTPS), y el frontend está en Vercel.

---

## Datos de referencia (no cambian)

| Cosa | Valor |
|---|---|
| Código local | `/Users/facu/documents/Facu/bingo/backend` |
| VM (usuario + IP) | `opc@64.181.182.200` |
| Clave SSH | `~/Downloads/bingo-vm-ssh.key` |
| Binario en la VM (el que corre) | `/usr/local/bin/bingo` |
| Carpeta de subida (scp) | `/home/opc/bingo` |
| Servicio systemd | `bingo.service` |
| Dominio público | `https://bingo-hanamaru.duckdns.org` |
| Endpoint de salud | `https://bingo-hanamaru.duckdns.org/health` |
| Origen CORS (front) | `https://bingo-frontend-mu.vercel.app` |
| Arquitectura de build | `linux / amd64` |

> ⚠️ **Usá la Terminal nativa de macOS, NO Ghostty.** Ghostty da problemas al pegar bloques y con `nano` (`xterm-ghostty`), y nos colgó la sesión más de una vez.

---

## TL;DR — Deploy rápido (cuando ya sabés lo que hacés)

Desde tu Mac, parado en la carpeta del backend:

```bash
cd /Users/facu/documents/Facu/bingo/backend

# 1. Compilar para Linux
GOOS=linux GOARCH=amd64 go build -o bingo .

# 2. Subir a la VM
scp -i ~/Downloads/bingo-vm-ssh.key bingo opc@64.181.182.200:/home/opc/

# 3. Entrar, instalar y reiniciar (en un solo SSH)
ssh -i ~/Downloads/bingo-vm-ssh.key opc@64.181.182.200 \
  'sudo cp /home/opc/bingo /usr/local/bin/bingo && \
   sudo chmod +x /usr/local/bin/bingo && \
   sudo restorecon -v /usr/local/bin/bingo && \
   sudo systemctl restart bingo && \
   sudo systemctl status bingo --no-pager'

# 4. Verificar desde tu Mac
curl https://bingo-hanamaru.duckdns.org/health
```

Esperás `{"status":"ok"}` y listo.

---

## Paso a paso detallado

### Paso 1 — Hacé tus cambios en el código

Editá lo que necesites en `/Users/facu/documents/Facu/bingo/backend`.

Antes de desplegar, **probá local** que compila y que los tests pasan:

```bash
cd /Users/facu/documents/Facu/bingo/backend
go build ./...        # compila todo, sin generar binario
go test -race ./...   # corre los tests con detector de race conditions
```

Si algo falla acá, arreglalo antes de seguir. No tiene sentido desplegar algo roto.

### Paso 2 — Compilá el binario para Linux

La VM es **x86/amd64** (shape `VM.Standard.E2.1.Micro`), así que hay que cross-compilar
desde tu Mac (que es ARM). Los `GOOS`/`GOARCH` son obligatorios:

```bash
cd /Users/facu/documents/Facu/bingo/backend
GOOS=linux GOARCH=amd64 go build -o bingo .
```

Esto genera un archivo `bingo` (~9 MB) en la carpeta. Verificá que se creó:

```bash
ls -lh bingo
```

### Paso 3 — Subí el binario a la VM

```bash
scp -i ~/Downloads/bingo-vm-ssh.key bingo opc@64.181.182.200:/home/opc/
```

Esto lo deja en `/home/opc/bingo` en la VM. (La advertencia sobre
"post-quantum key exchange" es inofensiva, ignorala.)

### Paso 4 — Instalá el binario en su lugar definitivo

Conectate a la VM:

```bash
ssh -i ~/Downloads/bingo-vm-ssh.key opc@64.181.182.200
```

Una vez adentro (prompt `[opc@bingo-vm ~]$`), copiá el binario a `/usr/local/bin`.

> 🔑 **¿Por qué `/usr/local/bin` y no `/home/opc`?**
> SELinux (activo en Oracle Linux 9) **bloquea** que un servicio systemd ejecute
> binarios que viven en el home de un usuario. Da el error `203/EXEC` /
> "Permission denied" aunque el archivo tenga permisos correctos. La solución es
> tenerlo en una ruta de sistema y aplicarle el contexto SELinux con `restorecon`.

```bash
sudo cp /home/opc/bingo /usr/local/bin/bingo
sudo chmod +x /usr/local/bin/bingo
sudo restorecon -v /usr/local/bin/bingo
```

### Paso 5 — Reiniciá el servicio

```bash
sudo systemctl restart bingo
sudo systemctl status bingo --no-pager
```

El `status` tiene que mostrar **`active (running)`** en verde.
(El `--no-pager` evita la pantalla rara de "terminal is not fully functional".)

Si querés ver los logs del backend:

```bash
sudo journalctl -u bingo --no-pager | tail -20
```

Deberías ver la línea:
`bingo-back listening on :8080 (CORS origin: https://bingo-frontend-mu.vercel.app)`

### Paso 6 — Verificá de punta a punta

Salí de la VM (`exit`) o abrí otra terminal en tu Mac, y probá el HTTPS real:

```bash
curl https://bingo-hanamaru.duckdns.org/health
```

Esperás: `{"status":"ok"}`

Esto prueba toda la cadena: Mac → DuckDNS → Caddy (HTTPS) → backend.
**Si devuelve eso, el deploy terminó.** No hace falta tocar Vercel ni nada más
(el frontend ya apunta a este dominio).

---

## Verás esto y está OK (no es error)

- **`curl http://64.181.182.200:8080/...` falla** → correcto y a propósito. El puerto
  8080 NO está abierto al público; solo Caddy (interno) lo usa. Probá siempre por
  `https://bingo-hanamaru.duckdns.org`.
- **`ping bingo-hanamaru.duckdns.org` da "Request timeout"** → normal. La VM no
  responde ICMP. El DNS igual resuelve bien a `64.181.182.200`.
- **Logs de Caddy con bots escaneando `/wp-json/`, `/xmlrpc.php`** → ruido de
  internet, inofensivo.
- **Advertencia SSH "post-quantum key exchange"** → inofensiva.

---

## Troubleshooting

### El servicio no arranca: `status=203/EXEC` / "Permission denied"
Es **SELinux** bloqueando el binario. Asegurate de que esté en `/usr/local/bin` y
corré `restorecon`:
```bash
sudo restorecon -v /usr/local/bin/bingo
sudo systemctl restart bingo
```
Si seguís peleando, limpiá el contador de reintentos y reintentá:
```bash
sudo systemctl reset-failed bingo
sudo systemctl restart bingo
```

### El servicio reinicia en loop ("restart counter is at N")
Es el `Restart=always` reintentando cada 3 s porque el binario falla al arrancar.
Mirá los logs para ver la causa real:
```bash
sudo journalctl -u bingo --no-pager | tail -30
```

### La VM se cuelga / SSH queda colgado en "Local version string..."
La VM tiene **poca RAM (~1 GB)**. Operaciones pesadas (sobre todo `dnf`) la ahogan.
El backend en sí es liviano (~1 MB de RAM), no es el problema.
- Reiniciá desde la consola de Oracle: Compute → Instances → `bingo-vm` →
  **Reboot**. Si queda trabada en "Stopping" más de unos minutos, usá **Force reboot**.
- **No encimes acciones**: disparás una, esperás a que el estado se asiente, y recién
  ahí la siguiente. Si tirás dos seguidas da "instance is currently being modified".
- Caddy, firewall, swap y el servicio `bingo` sobreviven el reboot (están todos
  `enable`/persistentes). Tras reiniciar, el backend arranca solo.

### Cambió la IP pública
La IP `64.181.182.200` es **efímera**: en un stop/start puede cambiar (en reboots
normales suele mantenerse). Si cambió:
1. Actualizá el registro en DuckDNS (poné la IP nueva en `bingo-hanamaru`).
2. Actualizá esta línea del README y tus comandos `ssh`/`scp`.
3. El frontend NO necesita cambios (apunta al dominio, no a la IP).

### Cambió la URL del frontend (afecta CORS)
Si redesplegás el front con otra URL de Vercel, hay que actualizar el `CORS_ORIGIN`
del backend, si no el navegador bloquea las llamadas. En la VM:
```bash
sudo sed -i 's|Environment=CORS_ORIGIN=.*|Environment=CORS_ORIGIN=https://NUEVA-URL.vercel.app|' /etc/systemd/system/bingo.service
sudo systemctl daemon-reload
sudo systemctl restart bingo
```
(Reemplazá `https://NUEVA-URL.vercel.app` por la URL real, sin barra final.)

---

## Anexo — Recrear el servicio systemd desde cero

Si alguna vez se pierde el archivo del servicio, recrealo línea por línea
(método a prueba de cuelgues, sin `nano` ni pegar bloques grandes):

```bash
echo '[Unit]' | sudo tee /etc/systemd/system/bingo.service
echo 'Description=Bingo backend' | sudo tee -a /etc/systemd/system/bingo.service
echo 'After=network.target' | sudo tee -a /etc/systemd/system/bingo.service
echo '' | sudo tee -a /etc/systemd/system/bingo.service
echo '[Service]' | sudo tee -a /etc/systemd/system/bingo.service
echo 'Type=simple' | sudo tee -a /etc/systemd/system/bingo.service
echo 'User=opc' | sudo tee -a /etc/systemd/system/bingo.service
echo 'WorkingDirectory=/home/opc' | sudo tee -a /etc/systemd/system/bingo.service
echo 'ExecStart=/usr/local/bin/bingo' | sudo tee -a /etc/systemd/system/bingo.service
echo 'Environment=CORS_ORIGIN=https://bingo-frontend-mu.vercel.app' | sudo tee -a /etc/systemd/system/bingo.service
echo 'Restart=always' | sudo tee -a /etc/systemd/system/bingo.service
echo 'RestartSec=3' | sudo tee -a /etc/systemd/system/bingo.service
echo '' | sudo tee -a /etc/systemd/system/bingo.service
echo '[Install]' | sudo tee -a /etc/systemd/system/bingo.service
echo 'WantedBy=multi-user.target' | sudo tee -a /etc/systemd/system/bingo.service

sudo systemctl daemon-reload
sudo systemctl enable bingo
sudo systemctl start bingo
sudo systemctl status bingo --no-pager
```

---

## Comandos útiles de referencia

```bash
# Estado del servicio
sudo systemctl status bingo --no-pager

# Logs (últimas 30 líneas)
sudo journalctl -u bingo --no-pager | tail -30

# Logs en vivo (Ctrl+C para salir)
sudo journalctl -u bingo -f

# Reiniciar / parar / arrancar
sudo systemctl restart bingo
sudo systemctl stop bingo
sudo systemctl start bingo

# Estado de Caddy (el proxy HTTPS)
sudo systemctl status caddy --no-pager
sudo journalctl -u caddy --no-pager | tail -30

# Ver el Caddyfile
cat /etc/caddy/Caddyfile

# Verificación de salud desde el Mac
curl https://bingo-hanamaru.duckdns.org/health
```