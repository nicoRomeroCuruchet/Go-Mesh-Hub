# Go-Mesh-Hub 🌐

![Go Version](https://img.shields.io/github/go-mod/go-version/nicoRomeroCuruchet/Go-Mesh-Hub)
![License](https://img.shields.io/badge/license-MIT-blue)
![Build Status](https://img.shields.io/badge/build-passing-brightgreen)
![Platform](https://img.shields.io/badge/platform-linux%2Famd64%20%7C%20linux%2Farm64-lightgrey)

**Go-Mesh-Hub** is a lightweight, secure, and highly scalable VPN orchestrator written in Go. It creates a virtual overlay network, enabling seamless bidirectional communication between devices behind restrictive NATs, CGNATs, and Firewalls without requiring manual port forwarding on the edge devices.

Designed for **IoT Fleets** (NVIDIA Jetson, Raspberry Pi), **Edge Computing**, and secure **Home Labs**.

![Dashboard Preview](docs/dashboard_preview.png)
*(Replace this path with your actual screenshot)*

## 🚀 Key Features

* **Hub & Spoke Topology:** Centralized signaling with efficient UDP tunneling.
* **Zero-Config Edge:** Clients (Agents) automatically traverse NATs using **UDP Hole Punching** and Keep-Alives.
* **Military-Grade Security:** All traffic is encrypted using **ChaCha20-Poly1305** (AEAD) with nonces to prevent replay attacks.
* **Real-Time Observability:** Embedded Web Dashboard for monitoring peer status, bandwidth usage (Rx/Tx), and latency.
* **Smart Build System:** Automated cross-compilation for Intel/AMD and ARM architectures via `setup.sh`.
* **Layer 3 Tunneling:** Uses a standard `TUN` interface, supporting ICMP (Ping), TCP (SSH, HTTP), and UDP.

## 🏗️ Architecture

```mermaid
graph TD
    User[Admin] -->|HTTP :8080| Dashboard[Web Dashboard]
    
    subgraph Cloud / Public Internet
        Hub[Go-Mesh-Hub Server]
    end
    
    subgraph Private Network A
        Jetson[Edge: Jetson Nano] -->|Encrypted UDP| Hub
    end
    
    subgraph Private Network B
        BBB[Edge: BeagleBone] -->|Encrypted UDP| Hub
    end
    
    Dashboard -.-> Hub
    Jetson <-->|Virtual IP 10.0.0.x| BBB
````

## 🛠️ Installation & Setup

### Prerequisites

  * **Linux OS** (Debian, Ubuntu, Arch, Alpine, etc.)
  * **Root privileges** (required to create network interfaces)

### Option A: Automated Setup (Recommended)

We provide a bootstrap script that detects your architecture, installs Go (if missing), and builds the binaries automatically.

1.  **Clone the repository:**

    ```bash
    git clone [https://github.com/nicoRomeroCuruchet/Go-Mesh-Hub.git](https://github.com/nicoRomeroCuruchet/Go-Mesh-Hub.git)
    cd Go-Mesh-Hub
    ```

2.  **Run the Setup Script:**

    ```bash
    chmod +x setup.sh
    ./setup.sh
    ```

3.  **Done\!** Binaries are located in the `bin/` directory.

### Option B: Manual Build (For Developers)

If you prefer using `Make`:

```bash
make build
# or
make clean
```

-----

## ⚙️ Deployment Guide

### 1\. Start the Hub (Server)

Deploy this on a machine with a **Public IP** or with **UDP Port 45678** forwarded.

```bash
# Run as root
sudo ./bin/hub \
  -local-port 45678 \
  -web-port 8080 \
  -tun-ip 10.0.0.1 \
  -secret "change-this-to-a-strong-password"
```

  * **UDP :45678**: VPN Traffic (Must be open to internet).
  * **TCP :8080**: Web Dashboard (Internal use).

### 2\. Start an Agent (Client)

Deploy this on your edge devices (Jetson, BBB, Laptops). No incoming ports needed.

```bash
# Replace <HUB_PUBLIC_IP> with your server's real IP
sudo ./bin/agent \
  -hub-ip <HUB_PUBLIC_IP> \
  -hub-port 45678 \
  -tun-ip 10.0.0.2 \
  -secret "change-this-to-a-strong-password"
```

### 3\. Verify Connectivity

From the Agent (`10.0.0.2`), you can now reach the Hub or other Agents:

```bash
# Ping the Hub
ping 10.0.0.1

# SSH into another device in the mesh
ssh user@10.0.0.3
```

## 📊 Monitoring Dashboard

The Hub includes a built-in web server. Access it via:

> **http://\<HUB\_LOCAL\_IP\>:8080**

It provides real-time stats:

  * **Peer Status:** Online / Lagging / Offline.
  * **Data Usage:** Real-time Rx/Tx counters.
  * **Last Seen:** Heartbeat tracking.

## 📂 Project Structure

This project adheres to the [Standard Go Project Layout](https://github.com/golang-standards/project-layout).

| Directory | Purpose |
| :--- | :--- |
| `cmd/hub` | Main entry point for the Server application. |
| `cmd/agent` | Main entry point for the Client application. |
| `internal/security` | AEAD Encryption wrapper (ChaCha20-Poly1305). |
| `internal/tun` | OS-level interactions (Syscalls, IOCTL) for the virtual interface. |
| `internal/router` | In-memory routing table logic and state management. |
| `internal/dashboard` | Embedded HTML templates and HTTP handlers. |
| `bin/` | Compiled binaries output location. |

## 🧪 Troubleshooting

**1. "Handshake failed" / Auth Error**
Ensure both Hub and Agent are using the **exact same** `-secret` string.

**2. Connection works but Ping fails**
Ensure IP Forwarding is enabled on the Hub if you want to route traffic to the internet (not just mesh):

```bash
sudo sysctl -w net.ipv4.ip_forward=1
```

**3. "Operation not permitted"**
The binary must be run with `sudo` because creating a `tun0` interface requires `CAP_NET_ADMIN` capabilities.

## 🤝 Contributing

Contributions are welcome\! Please open an issue first to discuss what you would like to change.

1.  Fork the Project
2.  Create your Feature Branch (`git checkout -b feature/AmazingFeature`)
3.  Commit your Changes (`git commit -m 'Add some AmazingFeature'`)
4.  Push to the Branch (`git push origin feature/AmazingFeature`)
5.  Open a Pull Request

## 📄 License

Distributed under the MIT License. See `LICENSE` for more information.
