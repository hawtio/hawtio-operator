#!/bin/bash

# Use the provided Makefile or default to 'Makefile'
MAKEFILE=${1:-Makefile}

printf "\nUsage:%-9s make \033[31m<PARAM1=val1 PARAM2=val2>\033[0m \033[36m<target>\033[0m\n" " "

echo
echo "Global Parameters (Defaults):"
echo

# Extract and format global parameters
awk '
# --- HELPER FUNCTION: Recursive Variable Expansion ---
function expand(val, iter, prev, match_start, match_len, ref_name, sub_val) {
  iter = 0
  # Safeguard: limit to 20 iterations to prevent infinite loops on circular references
  while (iter < 20) {
    prev = val
    # Look for $(VAR) or ${VAR}
    if (match(val, /\$[{(][A-Za-z0-9_]+[)}]/)) {
      match_start = RSTART
      match_len = RLENGTH
      # Extract just the variable name from inside the brackets
      ref_name = substr(val, match_start + 2, match_len - 3)

      # Lookup the value in our dictionary (or use empty string if not found)
      sub_val = (ref_name in dict) ? dict[ref_name] : ""

      # Substitute the resolved value back into the string
      val = substr(val, 1, match_start - 1) sub_val substr(val, match_start + match_len)
    }
    # If the string stopped changing, we are done expanding
      if (val == prev) break
      iter++
  }
  return val
}

# --- PARSER LOGIC ---

# Stop parsing if hitting the first real make target (lowercase letters followed by a colon)
# This prevents grabbing internal variables declared inside targets or later in the file.
/^[a-z0-9_-]+:/ { exit }

# Track conditional blocks. If we are inside an if/else block, skip the line.
/^ifeq/ || /^ifneq/ || /^ifdef/ || /^ifndef/ { inside_conditional = 1; next }
/^endif/ { inside_conditional = 0; next }
inside_conditional == 1 { next }

# Match ANY variable assignment (=, :=, ?=) to build our dictionary
/^[A-Z_]+[ \t]*[:?]?=/ {
  match($0, /[ \t]*[:?]?=[ \t]*/)
  var_name = substr($0, 1, RSTART-1)
  var_val = substr($0, RSTART+RLENGTH)
  operator = substr($0, RSTART, RLENGTH)

  # Save the value to our dictionary for future expansions
  dict[var_name] = var_val

  # If it uses ?=, add it to the printing queue (ignoring duplicates)
  if (index(operator, "?") > 0) {
    if (!seen[var_name]) {
      seen[var_name] = 1
      print_queue[++queue_len] = var_name
    }
  }
}

# --- FINAL EXECUTION ---
# Runs right before awk finishes (triggered by hitting the first make target)
END {
  for (i = 1; i <= queue_len; i++) {
    v = print_queue[i]
    # Expand the raw value using our helper function
    expanded_val = expand(dict[v])
    printf "%-15s \033[31m%-35s\033[93m %s\n", " ", v, expanded_val
  }
}
' "$MAKEFILE"

printf "\033[0m\nAvailable targets are:\n"

# Extract and format target documentation
awk '
/^#@[ \t]/ {
  printf "\033[36m%-15s\033[0m", $2; subdesc=0
  next
}

/^#==[ \t]/ {
  printf "\033[35m%s\033[0m\n", substr($0, 4)
  next
}

/^#===[ \t]/ {
  printf "%-14s \033[32m%s\033[0m\n", " ", substr($0, 5); subdesc=1
  next
}

# Print the "#* PARAMETERS:" header cleanly
/^#\*[ \t]/ {
  printf "\n%-15s \033[33m%s\033[0m\n", " ", substr($0, 4)
  next
}

# The new auto-wrapping parser for "#** PARAM_NAME: Description"
/^#\*\*/ {
    raw = substr($0, 5) # Strip the leading "#** "
    colon_pos = index(raw, ":") # Find the delimiter

    if (colon_pos > 0) {
      # Split into Parameter Name and Description
      param_name = substr(raw, 1, colon_pos)
      desc = substr(raw, colon_pos + 1)

      # Trim leading and trailing whitespace from the description
      sub(/^[ \t]+/, "", desc)
      sub(/[ \t]+$/, "", desc)

      desc_max_width = 60       # Wrap the description text after 60 chars

      # Print the indent and the parameter name in Red, padded to 35 spaces
      printf "%-16s\033[31m%-35s\033[0m ", " ", param_name

      # Turn on yellow for the parameter descriptions
      printf "\033[93m"
      # Word Wrap Logic
      n = split(desc, words, " ")
      line_len = 0
      for (i = 1; i <= n; i++) {
        w = words[i]
        # If adding the next word exceeds our max width, wrap to a new line
        if (line_len + length(w) > desc_max_width && line_len > 0) {
          # Print a newline, the 14-space indent, and an empty 35-space string to align the text
          printf "\n%-16s%-35s ", " ", ""
          line_len = 0
        }
        printf "%s ", w
        line_len += length(w) + 1
      }
      printf "\n"
    } else {
      # Fallback if someone forgets the colon
      printf "%-16s \033[31m%s\033[0m\n", " ", raw
    }
    next
}

/^#\*/ && (subdesc == 1) { printf "\n"; next }
/^#\-\-\-/ { printf "\n"; next }
' "$MAKEFILE"
