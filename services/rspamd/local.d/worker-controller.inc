# Web UI/API only on localhost; learning (rspamc learn_spam/ham) is allowed from localhost without password.
bind_socket = "127.0.0.1:11334";
secure_ip = "127.0.0.1";
# Only scan message bodies sent over the socket, never read files on request.
allow_file_and_shm_inputs = false;
