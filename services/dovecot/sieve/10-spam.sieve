require ["fileinto"];
# Rspamd adds "X-Spam: Yes" when a message scores above the add_header threshold.
if header :is "X-Spam" "Yes" {
  fileinto "Junk";
  stop;
}
