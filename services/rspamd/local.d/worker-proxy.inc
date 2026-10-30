# Milter for Postfix
bind_socket = "127.0.0.1:11332";
milter = yes;
timeout = 120s;
upstream "local" {
  default = yes;
  self_scan = yes;
}
# Only scan message bodies sent over the socket, never read files on request.
allow_file_and_shm_inputs = false;
