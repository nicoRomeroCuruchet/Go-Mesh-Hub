# Go-Mesh-Hub 🌐

![Go Version](https://img.shields.io/github/go-mod/go-version/nicoRomeroCuruchet/go-mesh-hub)
![License](https://img.shields.io/badge/license-MIT-blue)
![Build Status](https://img.shields.io/badge/build-passing-brightgreen)

**Go-Mesh-Hub** is a secure, high-performance VPN orchestrator written in Go. It establishes a virtual mesh network over UDP, enabling seamless connectivity between devices behind restrictive NATs and Firewalls (CGNAT) without requiring manual port forwarding on edge devices.

It employs a **Hub-and-Spoke** architecture for signaling and routing, designed specifically for IoT fleets (NVIDIA Jetson, Raspberry Pi) and Edge Computing.

## 🚀 Key Features

* **Zero-Config Connectivity:** Clients initiate connections using UDP Hole Punching / Keep-Alives to traverse NATs.
* **Military-Grade Security:** All traffic is encrypted using **ChaCha20-Poly1305** (AEAD) with Replay Attack protection via nonces.
* **Real-Time Dashboard:** Embedded Web UI for monitoring peer status, bandwidth usage (Rx/Tx), and latency.
* **Lightweight:** Compiles to a single static binary (~10MB) with minimal memory footprint.
* **Cross-Platform:** Runs on any Linux kernel with TUN/TAP support (Debian, Ubuntu, Arch, Alpine).

## 🏗️ Architecture

The system operates on Layer 3 (IP Layer) using a virtual TUN interface.

```mermaid
graph TD
    User[Administrator] -->|HTTP :8080| Dashboard[Web Dashboard]
    subgraph Public Internet
        Hub[Go-Mesh-Hub Server]
    end
    subgraph Private LAN A
        Jetson[Edge Device: Jetson] -->|UDP Encrypted| Hub
    end
    subgraph Private LAN B
        BBB[Edge Device: BBB] -->|UDP Encrypted| Hub
    end
    
    Dashboard -.-> Hub
    Jetson <-->|Virtual IP 10.0.0.x| BBB
