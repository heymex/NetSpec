# Cisco IOS-XE gNMI Configuration Guide for NetSpec

This guide provides step-by-step instructions for configuring gNMI (gRPC Network Management Interface) on Cisco IOS-XE devices to work with NetSpec.

## Prerequisites

- Cisco IOS-XE device running version **17.x or later** (recommended for full gNMI support)
- Administrative access to the device (enable mode)
- Network connectivity between NetSpec and the target device
- Understanding of Cisco IOS-XE configuration

## Minimum IOS-XE Version Requirements

- **Minimum**: IOS-XE 16.12.1 (basic gNMI support)
- **Recommended**: IOS-XE 17.x or later (full OpenConfig model support)
- **Required Features**: 
  - gNMI server capability
  - OpenConfig YANG models
  - NETCONF/YANG support

## Step 1: Verify gNMI Support

First, verify that your device supports gNMI:

```cisco
show platform software yang-management process
```

Look for `gnmi` in the output. If it's not present, you may need to enable it or upgrade your IOS-XE version.

## Step 2: Enable gNMI Server

### Basic Configuration (Insecure - Testing Only)

For initial testing and development, you can enable gNMI without TLS:

```cisco
configure terminal

! Enable gNMI server
gnmi-yang
 gnmi-yang server
 gnmi-yang port 9339

! Enable NETCONF/YANG (required for OpenConfig models)
netconf-yang

end
write memory
```

**⚠️ Security Warning**: This configuration uses an insecure connection. Use only for testing in isolated networks.

### Production Configuration (TLS Recommended)

For production deployments, enable TLS encryption:

```cisco
configure terminal

! Enable gNMI server with TLS
gnmi-yang
 gnmi-yang server
 gnmi-yang secure-server
 gnmi-yang secure-trustpoint <trustpoint-name>
 gnmi-yang secure-port 9339
 gnmi-yang port 9339  ! Optional: keep insecure port for testing

! Enable NETCONF/YANG
netconf-yang

end
write memory
```

**Note**: You must have a valid trustpoint configured before enabling secure-server. See the "TLS Certificate Setup" section below.

## Step 3: Create Monitoring User

Create a dedicated user account for NetSpec with appropriate privileges:

```cisco
configure terminal

! Create user with privilege level 15 (full access)
username netspec-monitor privilege 15 secret <strong-password>

! Optional: Create a more restricted user with only read access
! username netspec-reader privilege 1 secret <password>
! privilege exec level 1 show

end
write memory
```

**Security Best Practices**:
- Use a strong, unique password
- Consider using AAA (TACACS+/RADIUS) for centralized authentication
- Limit user privileges to minimum required (read-only if possible)
- Rotate passwords regularly

## Step 4: Configure Access Control (Optional but Recommended)

Restrict gNMI access to specific source IP addresses:

```cisco
configure terminal

! Create access list for NetSpec server
ip access-list standard GNMI-NETSPEC-ACL
 permit 10.0.0.100  ! Replace with NetSpec server IP
 permit 10.0.0.101  ! Add additional NetSpec server IPs if needed
 deny any log

! Apply ACL to gNMI (if supported on your IOS-XE version)
! Note: ACL application method may vary by IOS-XE version
! Check your specific version documentation

end
write memory
```

## Step 5: Verify gNMI Configuration

### Check gNMI Server Status

```cisco
show gnmi-yang server status
```

Expected output should show:
- Server status: `Running`
- Port: `9339` (or your configured port)
- Secure mode: `Enabled` (if TLS configured)

### Verify NETCONF/YANG

```cisco
show netconf-yang datastores
show netconf-yang sessions
```

### Test gNMI Connection (from NetSpec Server)

Using `gnmic` CLI tool (install from https://gnmic.openconfig.net/):

```bash
# Test capabilities
gnmic -a <device-ip>:9339 -u netspec-monitor -p <password> --insecure \
  capabilities

# Subscribe to interface state (test subscription)
gnmic -a <device-ip>:9339 -u netspec-monitor -p <password> --insecure \
  subscribe --path "/interfaces/interface/state/oper-status"
```

If the connection succeeds, you should see interface state updates.

## Step 6: TLS Certificate Setup (Production)

For production deployments, configure TLS certificates:

### Option 1: Self-Signed Certificate (Internal CA)

```cisco
configure terminal

! Generate self-signed certificate
crypto pki trustpoint NETSPEC-TRUSTPOINT
 enrollment selfsigned
 subject-name CN=<device-hostname>
 revocation-check none
 rsakeypair NETSPEC-KEY

! Generate the certificate
crypto pki enroll NETSPEC-TRUSTPOINT

! Configure gNMI to use the trustpoint
gnmi-yang
 gnmi-yang secure-trustpoint NETSPEC-TRUSTPOINT

end
write memory
```

### Option 2: CA-Signed Certificate (Recommended for Production)

```cisco
configure terminal

! Configure trustpoint for CA-signed certificate
crypto pki trustpoint NETSPEC-TRUSTPOINT
 enrollment url http://<ca-server>/certsrv/mscep/mscep.dll
 subject-name CN=<device-hostname>
 revocation-check crl
 rsakeypair NETSPEC-KEY

! Enroll with CA
crypto pki authenticate NETSPEC-TRUSTPOINT
crypto pki enroll NETSPEC-TRUSTPOINT

! Configure gNMI
gnmi-yang
 gnmi-yang secure-trustpoint NETSPEC-TRUSTPOINT

end
write memory
```

## Step 7: Configure NetSpec

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
GNMI_USERNAME=netspec-monitor
GNMI_PASSWORD=your-password-here
```

## Troubleshooting

### gNMI Connection Fails

**Symptom**: NetSpec cannot connect to device

**Solutions**:
1. Verify gNMI is enabled:
   ```cisco
   show gnmi-yang server status
   ```

2. Check firewall/ACL rules blocking port 9339

3. Verify credentials:
   ```bash
   gnmic -a <device-ip>:9339 -u <username> -p <password> --insecure capabilities
   ```

4. Check device logs:
   ```cisco
   show logging | include gnmi
   ```

### No Telemetry Data Received

**Symptom**: Connection succeeds but no interface state updates

**Solutions**:
1. Verify OpenConfig models are available:
   ```cisco
   show platform software yang-management process
   ```

2. Check if interfaces exist:
   ```bash
   gnmic -a <device-ip>:9339 -u <username> -p <password> --insecure \
     get --path "/interfaces/interface"
   ```

3. Verify subscription path is correct:
   - NetSpec subscribes to: `/interfaces/interface[name=*]/state/oper-status`
   - Ensure your IOS-XE version supports this path

### TLS Certificate Errors

**Symptom**: Connection fails with certificate errors

**Solutions**:
1. Verify trustpoint is configured:
   ```cisco
   show crypto pki trustpoints
   ```

2. Check certificate validity:
   ```cisco
   show crypto pki certificates
   ```

3. For testing, use `--insecure` flag in NetSpec (not recommended for production)

4. Ensure NetSpec server trusts the device certificate (or use self-signed with proper CA)

### Permission Denied Errors

**Symptom**: Authentication succeeds but operations fail

**Solutions**:
1. Verify user has sufficient privileges:
   ```cisco
   show privilege
   show users
   ```

2. Check if user needs privilege level 15 for gNMI operations

3. Verify AAA configuration isn't restricting access

## Security Considerations

### Production Recommendations

1. **Always use TLS** in production environments
2. **Use strong passwords** or integrate with AAA (TACACS+/RADIUS)
3. **Restrict source IPs** using ACLs if possible
4. **Monitor gNMI access** in device logs
5. **Rotate credentials** regularly
6. **Use read-only accounts** if NetSpec only needs to monitor (not configure)
7. **Keep IOS-XE updated** to latest stable version for security patches

### Network Security

- Place NetSpec server and network devices on a management network
- Use VPN or encrypted tunnels for remote monitoring
- Implement network segmentation to limit exposure
- Monitor for unauthorized gNMI connection attempts

## OpenConfig Paths Used by NetSpec

NetSpec subscribes to the following OpenConfig paths:

- **Interface Operational State**: `/interfaces/interface[name=*]/state/oper-status`
- **Interface Admin State**: `/interfaces/interface[name=*]/state/admin-status`
- **Port-Channel Members**: `/interfaces/interface[name=*]/aggregation/state/member`
- **BGP Neighbors**: `/network-instances/network-instance/protocols/protocol/bgp/neighbors/neighbor/state`
- **HSRP State**: `/Cisco-IOS-XE-hsrp-oper:hsrp-oper-data/hsrp-entry` (Cisco-specific)
- **Hardware Sensors**: `/components/component[class=FAN|TEMPERATURE_SENSOR|POWER_SUPPLY]/state`

Ensure your IOS-XE version supports these OpenConfig models.

## Example Complete Configuration

Here's a complete example configuration for a production-ready setup:

```cisco
! Enable gNMI with TLS
configure terminal

gnmi-yang
 gnmi-yang server
 gnmi-yang secure-server
 gnmi-yang secure-trustpoint NETSPEC-TRUSTPOINT
 gnmi-yang secure-port 9339

netconf-yang

! Create monitoring user
username netspec-monitor privilege 15 secret <strong-password>

! Optional: Configure AAA for centralized auth
! aaa new-model
! aaa authentication login default group tacacs+ local
! aaa authorization exec default group tacacs+ local

end
write memory
```

## Additional Resources

- [Cisco IOS-XE gNMI Configuration Guide](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/prog/configuration/173/b_173_programmability_cg/gnmi_yang.html)
- [OpenConfig Models](https://github.com/openconfig/public)
- [gNMI Specification](https://github.com/openconfig/reference/blob/master/rpc/gnmi/gnmi-specification.md)
- [gnmic CLI Tool](https://gnmic.openconfig.net/)

## Support

If you encounter issues not covered in this guide:

1. Check device logs: `show logging | include gnmi`
2. Verify IOS-XE version compatibility
3. Test with `gnmic` CLI tool to isolate NetSpec-specific issues
4. Review NetSpec logs for detailed error messages
