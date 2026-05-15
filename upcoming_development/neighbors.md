New PR: LLDP/CDP integration.

**Specification:** [neighbors-spec.md](./neighbors-spec.md)  
**Implementation PR:** Phase 1 — SNMP discovery wizard + `neighbor_rules` + Graphviz DOT export.

# 1. Use "rules" framework to identify how ports should be configured for certain devices
- Example: identify IP phones on the network via LLDP, verify that matching ports are assigned to the proper VLAN (listed in rules.yaml), and identify if any ports with IP phones are mislabeled in the IOS configuration
- Example: identify wireless APs on the network via LLDP, verify that matching ports are configured as 802.1q trunks, and identify if any ports with IP phones are mislabeled in the IOS configuration

# 2. Leverage neighbor info via streaming telemetry if possible
- Example: alert that a new AP is connected to a port based on streaming LLDP neighbor info

# 3. Leverage neighbor info via SNMP for discovery
- During the discovery process, walk neighbors in addition to ports, and present that information in the discovery wizard
- Reference rules.yaml to use neighbor discovery as a sorting method (see #1)

# 4. Build a graph tree with the discovered neighbor data
- As a reporting feature, build a graph tree of the network based on discovered neighbor and port description data
- Offer visualization of the tree with GraphViz
- Use this data as the foundation for symmetric mapping of uplink/downlink ports between switches, to understand parent-child dependencies within the network.
- Flag single points of failure or risky connections within the network based on understanding of upstream/downstream links. e.g., fully redundant links between access and distribution are put at risk by a lack of redundancy between that distribution switch and the core