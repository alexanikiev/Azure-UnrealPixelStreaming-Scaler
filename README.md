# Azure-UnrealPixelStreaming-Scaler

## Important Reference

Azure-UnrealPixelStreaming-Scaler project is a logical continuation of Azure-PixelStreamingCopilot-Sample project [here](https://github.com/alexanikiev/Azure-PixelStreamingCopilot-Sample) for developing and deploying VMSS custom Scaler written in Go using Azure Go SDK powering secure Unreal Azure Pixel Streaming at scale and on budget in Microsoft Azure Cloud.

## Welcome

Welcome to the Azure Unreal Pixel Streaming Scaler repository on GitHub. This code base written in Go uses Azure Go SDK [here](https://github.com/Azure/azure-sdk-for-go) for implementing Unreal Azure Pixel Streaming at scale and on budget in Microsoft Azure Cloud. When the goal is to make possible to stream high definition and high quality interactive 3D content via Epic Games' Unreal Pixel Streaming Technology at real-time on-demand in the web browser of your choice (Microsoft Edge, Google Chrome, Apple Safari, etc.) on the device of your choice (PC/Mac, tablet/iPad, phone/iPhone, etc.) over the Internet, there's a number of things top of mind amongst others:

1. Security
2. Scale & Performance
3. Budget

Unreal Pixel Streaming Scaler Reference Architecture and Reference Implementation aims to address the above aspects for deploying secure Unreal Pixel Streaming solutions in Microsoft Azure at scale and on budget.

## Motivation

Make everything as simple as possible, but not simpler.

## Getting Started

The bigger project is develop a web site with embedded Unreal Pixel Streaming experience in the web browser. The concrete task is develop Azure Unreal Pixel Streaming Scaler which will dynamically reprovision new VMSS instances with Unreal for pixel streaming over time.

Solution architecture of this template is described in detail in the following Medium articles:

* [TBD](https://alexanikiev.medium.com/) (Medium article on "How we built Arina's Story in Azure Cloud" will be published soon) 

Solution demo videos and technical walkthroughs are available on YouTube:

* [Arina's Story Azure Unreal Pixel Streaming Scaler (Warm Start)](https://www.youtube.com/watch?v=oYCH9k3_zzI)
* [Arina's Story](https://www.youtube.com/watch?v=Y4-QlYoimrQ)
* [Arina's Story Azure Unreal Pixel Streaming on Mobile](https://www.youtube.com/watch?v=cCucixTf4hE)

## Prerequisites

| Prerequisite                              | Link                                                 |
|-------------------------------------------|---------------------------------------------------------------|
| Azure DevOps Pipelines (or GitHub Actions)| https://azure.microsoft.com/products/devops/                  |
| Azure PaaS                                | https://portal.azure.com/                                     |
| Unreal Engine                             | https://www.unrealengine.com/en-US/download                   |
| Unreal Pixel Streaming Infrastructure     | https://github.com/EpicGamesExt/PixelStreamingInfrastructure  |
| TURN Server (coturn)                      | https://github.com/coturn/coturn                              |

## Solution Architecture

![Solution Architecture](/docs/images/scaler_high_level_architecture.png)

![Logical Architecture](/docs/images/scaler_logical_architecture.png)

![State Machine Pattern](/docs/images/scaler_state_machine_pattern.png)

## Deployment Architecture

![Deployment Architecture](/docs/images/scaler_deployment_architecture.png)

![Routing Schema](/docs/images/scaler_routing_schema.png)

## Security Architecture

[PixelStreamingInfrastructure/Docs/Security-Guidelines.md at master · EpicGamesExt/PixelStreamingInfrastructure](https://github.com/EpicGamesExt/PixelStreamingInfrastructure/blob/master/Docs/Security-Guidelines.md)

To enhance the security of your Pixel Streaming deployments, it is wise to implement additional measures for protection. This documentation page aims to provide you with valuable recommendations and suggestions to bolster the security of your deployments. By following these guidelines, you can significantly enhance the overall security posture and safeguard your Pixel Streaming environment effectively.

### Tips to Improve Security

Please note that implementing the following suggestions may introduce additional setup complexity and could result in increased latency.
1.  **Isolate Unreal Engine Instance:** Avoid deploying the Unreal Engine instance on a cloud machine with a public IP. Instead, only allowlist the necessary servers, such as the signalling and TURN servers, to communicate with the UE instance.
    
2.  **Route Media Traffic through TURN Server:** For enhanced security, enforce routing all media traffic through the TURN server. By doing so, only the TURN server and signalling server will be permitted to communicate with the UE instance. Keep in mind that this approach may introduce some additional latency.
    
3.  **Secure TURN Server with User Credentials:** Configure the TURN server with a user database and assign unique credentials to each user. This additional security layer prevents unauthorized access to the relay. By default, Pixel Streaming employs the same TURN credentials for every session, which may simplify access for potential attackers.
    
4.  **Avoid Storing Important Credentials in the UE Container:** As a precautionary measure, refrain from storing any critical credentials or sensitive information within the UE container. This practice helps maintain a higher level of security.
    
5.  **Disable Pixel Streaming Console Commands:** Pixel Streaming ensures that all media traffic is encrypted end-to-end, guaranteeing secure communication. However, note that Pixel Streaming allows users to send commands to the UE instance if enabled. To eliminate this possibility, launch without the `-AllowPixelStreamingCommands` flag.
    
6.  **Separate TURN and Signalling Servers:** It is recommended to avoid colocating the TURN and signalling servers with the UE instance on the same IP or virtual machine (VM). This enables you to configure separate ingress/egress security policies for each server, allowing flexibility in defining the desired level of strictness or looseness. For example, the TURN server can have more relaxed policies while the UE instance can have stricter ones.
    
By following these tips, you can enhance the security of your Pixel Streaming setup and mitigate potential risks.

## Performance Benchmarking

### Unreal Rendering Performance (FPS)

Depending on the type of GPU compute used Unreal rendering performance will vary. We benchmarked Unreal rendering performance with NV-series and NG-series GPU compute SKUs with Unreal app containing complex lighting and sophisticated 3D structures and objects including Meta-human models. We measured performance with FPS (frames per second).

![Unreal Rendering Performance](/docs/images/scaler_benchmarking_rendering_performance_nv_vs_ng.png)

The goal was to have minimum 30fps, ideally 45-60fps for real-time pixel streaming rendering performance for the experience.

The following FPS ranges determine different tiers of experience:
- Less than 30fps: May be laggy and slow
- 30-40fps: Playable and responsive
- 45-60fps: Smooth production grade for Meta-human
- More than 60fps: Ideal but uncommon unless super optimized

GPU utilization profile for NV-series GPU compute can be found [here](/docs/images/scaler_benchmarking_rendering_performance_nv.png)

GPU utilization profile for NG-series GPU compute can be found [here](/docs/images/scaler_benchmarking_rendering_performance_ng.png)

### TURN Relay Performance (RTT)

Depending on how connectivity gets established for pixel streaming the latency will vary. For example, in case of firewall or NAT restrictions TURN server will be required, otherwise a peer-to-peer direct connection will be established via STUN. We benchmarked latency with and without TURN server, we also considered using local TURN server vs external TURN server for comparison. We measured performance with RTT (rount trip time). RTT includes the following communication chain when TURN is involved: Client <-> TURN <-> Unreal <-> TURN <-> Client; and Client <-> Unreal (via STUN (P2P)) when TURN is not needed.

**Local TURN**
![Local TURN](/docs/images/scaler_local_turn_performance.png)

``$peerConnectionOptions = "{ \""iceServers\"": [{\""urls\"": [\""turn:" + $global:TurnServer + "\""], \""username\"": \""abc\"", \""credential\"": \""xyz\""}], \""iceTransportPolicy\"": \""relay\"" }"``

**External TURN (`metered.ca`)**
![External TURN](/docs/images/scaler_external_turn_performance.png)

``$peerConnectionOptions = "{ \""iceServers\"": [{\""urls\"": [\""turn:na.relay.metered.ca:80\""], \""username\"": \""abc\"", \""credential\"": \""xyz\""}, {\""urls\"": [\""turn:na.relay.metered.ca:80?transport=tcp\""], \""username\"": \""abc\"", \""credential\"": \""xyz\""}, {\""urls\"": [\""turn:na.relay.metered.ca:443\""], \""username\"": \""abc\"", \""credential\"": \""xyz\""}, {\""urls\"": [\""turns:na.relay.metered.ca:443?transport=tcp\""], \""username\"": \""abc\"", \""credential\"": \""xyz\""}], \""iceTransportPolicy\"": \""relay\"" }"``

**No TURN, STUN (P2P)**
![No TURN, STUN (P2P)](/docs/images/scaler_stun_p2p_performance.png)

``$peerConnectionOptions = "{ \""iceServers\"": [{ \""urls\"": [\""stun:$($global:StunServer)\""] }, { \""urls\"": [\""turn:$($global:TurnServer)\""], \""username\"": \""abc\"", \""credential\"": \""xyz\""}] }"``

The goal was to be in the vicinity of 100 ms for near-real-time interaction for the experience.

The following RTT ranges determine different tiers of experience:
- Less than 50 ms: Ideal, ultra-responsive, LAN-like experience
- 50–150 ms: Good, smooth and playable, near-real-time interaction
- 150–250 ms: Acceptable, noticeable input delay, still usable
- 250–400 ms: Degraded, sluggish response, video may stutter
- More than 400 ms: Unacceptable, high latency, poor interactivity, bad for gameplay or UI

Note: In our benchmarking using local TURN sometimes showed better results than using direct STUN (P2P) connection without TURN, however, this is not non-typical due to specificity of networking and other Azure setups. In any case it does not mean that TURN is faster in general.

### Warm Start vs Cold Start Performance

The necessity to strike the right balance between performance and cost of Unreal Pixel Streaming implementation in Azure led to a design choice of warm pool and cold pool of VMSS instances. Specifically, VMs in warm pool would enjoy much faster start time at the expense of being more constly (Stopped status) because of GPU-compute continuous cost, and VMs in cold pool would not incur GPU-compute cost until started (Stopped/Deallocated status) but the start time would be significanly slower.

We benchmarked the start time for warm pool and cold pool VMs in different regions in Azure:

1. Warm start: ~ 5 seconds on average to start the VM (Stopped -> Started status)
2. Cold start: ~ 100 seconds on average to start the VM (Stopped/Deallocated -> Started status)

## Disclaimer

This code is provided "as is" without warranties to be used at your own risk. Parts of the code openly available on Internet are subject for copyright by Microsoft and Epic Games and marked as such inline.
