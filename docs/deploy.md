# Deploy

Primary host, quadlet `deploy/openpak-x360-xmatch.container` (IP pinned in openpak.network),
Traefik routes from `deploy/traefik.yml` merged into the shared dynamic file. The console reaches
it only once service 6 is in the KDC's D7 list (`X360_KDC_D7_SERVICES`); relaunch/re-sign-in
after changing that list, since the list comes from the boot-time ticket.
