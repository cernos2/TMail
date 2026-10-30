#!/bin/sh
# Text of an attachment for the full-text index (Dovecot fts_decoder): content type in $1, the file
# on stdin, plain UTF-8 text on stdout. Quick tools only (no LibreOffice): PDF text layer, Word,
# Excel, OpenDocument, RTF, CSV. Scans without a text layer give nothing.
# Called without arguments, Dovecot asks which content types we handle ("type extension" lines,
# ended by an empty line; the extension maps application/octet-stream attachments by file name).
if [ -z "$1" ]; then
  cat <<'FORMATS'
application/pdf pdf
application/x-pdf pdf
application/msword doc
application/vnd.ms-excel xls
application/rtf rtf
text/rtf rtf
application/vnd.openxmlformats-officedocument.wordprocessingml.document docx
application/vnd.openxmlformats-officedocument.spreadsheetml.sheet xlsx
application/vnd.openxmlformats-officedocument.presentationml.presentation pptx
application/vnd.ms-word.document.macroenabled.12 docm
application/vnd.ms-excel.sheet.macroenabled.12 xlsm
application/vnd.oasis.opendocument.text odt
application/vnd.oasis.opendocument.spreadsheet ods
application/vnd.oasis.opendocument.presentation odp
text/csv csv

FORMATS
  exit 0
fi
ct=$(printf '%s' "$1" | tr 'A-Z' 'a-z' | cut -d';' -f1)
tmp=$(mktemp -d) || exit 0
trap 'rm -rf "$tmp"' EXIT
f="$tmp/in"
head -c 30000000 > "$f"   # bigger files are not read for the index
# Generic types (octet-stream…): look at the content itself.
case "$ct" in
  application/octet-stream|application/x-zip-compressed|application/zip|binary/*|application/unknown|application/force-download)
    ct=$(file -b --mime-type "$f")
    if [ "$ct" = application/zip ] || [ "$ct" = application/octet-stream ]; then
      names=$(unzip -Z1 "$f" 2>/dev/null | head -50)
      case "$names" in
        *word/document.xml*) ct=application/vnd.openxmlformats-officedocument.wordprocessingml.document ;;
        *xl/workbook.xml*) ct=application/vnd.openxmlformats-officedocument.spreadsheetml.sheet ;;
        *content.xml*) ct=application/vnd.oasis.opendocument.text ;;
      esac
    fi ;;
esac
# XML → text: tags become spaces, the common entities are undone.
xmltext() { sed -e 's/<[^>]*>/ /g' -e 's/&lt;/</g' -e 's/&gt;/>/g' -e 's/&quot;/"/g' -e "s/&apos;/'/g" -e 's/&amp;/\&/g' | tr -s ' \t\r\n' ' '; }
run() { timeout 60 "$@" 2>/dev/null; }
case "$ct" in
  application/pdf|application/x-pdf) run pdftotext -q -enc UTF-8 -l 50 "$f" - ;;
  application/msword) run catdoc -d utf-8 -w "$f" ;;
  application/vnd.ms-excel) run xls2csv -d utf-8 "$f" ;;
  application/rtf|text/rtf) run catdoc -d utf-8 -w "$f" ;;
  application/vnd.openxmlformats-officedocument.wordprocessingml.*|application/vnd.ms-word.document.*)
    run unzip -p "$f" 'word/document.xml' 'word/header*.xml' 'word/footer*.xml' | xmltext ;;
  application/vnd.openxmlformats-officedocument.spreadsheetml.*|application/vnd.ms-excel.sheet.*)
    run unzip -p "$f" 'xl/sharedStrings.xml' 'xl/worksheets/*.xml' | xmltext ;;
  application/vnd.openxmlformats-officedocument.presentationml.*)
    run unzip -p "$f" 'ppt/slides/*.xml' | xmltext ;;
  application/vnd.oasis.opendocument.*) run unzip -p "$f" content.xml | xmltext ;;
  text/csv|text/plain|text/tab-separated-values) cat "$f" ;;
esac
exit 0
