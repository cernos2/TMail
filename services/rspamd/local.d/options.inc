# Own recursive resolver: public resolvers are refused by DNS blocklists such as Spamhaus.
dns {
  nameserver = ["127.0.0.1:8053"];
}

# The antivirus may need up to 90 s for a big letter (clamav timeout in antivirus.conf);
# the whole check must not be cut off before it.
task_timeout = 120s;
