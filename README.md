# Balboa Discovery Proxy

This is intended for use by people with a spa using the Balboa WiFi module who
place their IoT devices on a separate network but still want the "Local" option
in the Balboa app to continue operating.

This works by responding to discovery broadcasts from the app and then
forwarding connections to the spa module's actual IP address. This works best
if the spa module has a fixed IP address because discovery of the spa is not
supported.

The app connects to the IP address that responds to the discovery broadcast so
it is not possible to arrange for the app to connect directly to the spa without
significant additional effort.

The only required parameter is the destination address of the spa module, but
filling in the discovery MAC address (MAC address of the spa module) is strongly
recommended because the app displays the address and it may have unknown effects
such as influencing the cloud login process.
