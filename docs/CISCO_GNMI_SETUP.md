# Cisco IOS-XE Telemetry Setup Guide for NetSpec

This guide documents the current NetSpec operating model for Cisco IOS-XE: push telemetry ingest plus SNMP-targeted validation.

## Current NetSpec Telemetry Model

NetSpec currently supports:
- `snmp_validate_only`
- `telemetry_ingest_push`

`gnmi_pull` is no longer supported.

## Recommended Deployment Pattern

Switches stream telemetry out to a receiver, and NetSpec validates state with targeted SNMP GETs. This avoids relying on unstable inbound gNMI server behavior on IOS-XE.

## Prerequisites

- Cisco IOS-XE device running version **17.x or later**
- Administrative access to the device (enable mode)
- Network connectivity between NetSpec and the target device
- SNMPv2c community configured on the device and in NetSpec `.env` (`SNMP_COMMUNITY`)

## Configure NetSpec

Update your NetSpec `config/desired-state.yaml` with the device information:

```yaml
devices:
  my-switch:
    address: 10.0.0.1          # Device IP address
    description: "Core Switch - Building A"
    # credentials_ref: custom_creds  # Optional: use custom credentials
    
    interfaces:
      GigabitEthernet1/0/1:
        description: "Uplink to Distribution"
        desired_state: up
        admin_state: enabled
        alerts:
          state_mismatch: critical
```

Set environment variables in your `.env` file:

```bash
SNMP_COMMUNITY=your-snmp-community
```

Use telemetry mode explicitly in `config/desired-state.yaml`:

```yaml
global:
  telemetry_mode: snmp_validate_only   # or telemetry_ingest_push
  snmp:
    version: 2c
    port: 161
    community_env: SNMP_COMMUNITY
    validation_interval: 10s
    timeout: 3s
    retries: 1
```

## Configure IOS-XE Dial-Out Telemetry (Push)

Configure model-driven telemetry dial-out from switch to your receiver:

```cisco
configure terminal
!
telemetry ietf subscription 100
 encoding encode-kvgpb
 stream yang-push
 update-policy periodic 1000
 filter xpath /interfaces/interface/state
 receiver ip address <receiver-ip> 57500 protocol grpc-tcp
!
end
write memory
```

Notes:
- Exact telemetry CLI can vary by IOS-XE train/platform; use `telemetry ietf ?` and `subscription ?` to discover valid syntax.
- Start with a narrow XPath/filter and low receiver count, then expand.
- Keep SNMP targeted validation enabled in NetSpec to confirm telemetry events.
- NetSpec `telemetry_ingest_push` currently accepts newline-delimited JSON on `tcp/57500`; use a lightweight translator/collector to convert IOS-XE `grpc-tcp` telemetry into NetSpec ingest events.
- In isolated management VRF/firewall environments, token-based payload auth can be omitted and transport isolation can be the primary control.

## Troubleshooting

### No Telemetry Data Received

**Symptom**: Connection succeeds but no interface state updates

**Solutions**:
1. Verify telemetry subscriptions on device:
   ```cisco
   show telemetry ietf subscription all
   ```

2. Verify receiver IP/port is reachable from switch management VRF.
3. Confirm NetSpec `global.ingest.port` matches the translator destination.

## Security Considerations

### Production Recommendations

1. Restrict telemetry receiver access to trusted management networks.
2. Restrict SNMP access with ACLs and source controls.
3. Rotate SNMP communities and API tokens periodically.
4. Keep IOS-XE updated to stable releases with security fixes.

### Network Security

- Place NetSpec server and network devices on a management network
- Use VPN or encrypted tunnels for remote monitoring
- Implement network segmentation to limit exposure
- Monitor for unauthorized telemetry/collector access attempts

## Additional Resources

- [Cisco IOS-XE Telemetry Programming Guide](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/prog/configuration/173/b_173_programmability_cg/gnmi_yang.html)
- [OpenConfig Models](https://github.com/openconfig/public)

## Support

If you encounter issues not covered in this guide:

1. Check device logs for telemetry receiver and stream status
2. Verify IOS-XE version compatibility
3. Review NetSpec logs for ingest and SNMP validation errors
