#!/bin/bash

# TODO automate
GO_VERSION=1.24.6

FAIL_LOG="go-get-upgrade.fail.log"
if [ -f ${FAIL_LOG} ]; then
  rm ${FAIL_LOG}
fi

go list -mod=mod -u -m -json all | \
jq -r 'select(.Main != true and .Update != null) | [.Path, .Version, .Update.Version, .Deprecated] | @tsv' | \
while IFS=$'\t' read -r path curr new deprecated
do
  if [ -z "${path}" ]; then
    echo "  Error: Cannot process ${dep}"
    echo
    continue
  fi

  deprecated=$(echo "${dep}" | jq -r '.Deprecated | select( . != null )')
  if [ -n "${deprecated}" ]; then
    echo "Error: ${path} is deprecated. Please upgrade manually."
    echo "Deprecated ---> ${path}" >> ${FAIL_LOG}
    echo
    continue
  fi

  echo "Dependency to be updated: ${path}"
  echo "Updating ${path}: ${curr} ---> ${new}"

  # Stops the version of go installed from being automatically updated
  export GOTOOLCHAIN=local
  result=$(go get -t -u "${path}@${new}" 2>&1)
  if [ "${?}" != "0" ]; then
    echo  "Error: ${path} failed to update. Please upgrade manually."
    echo "Failed to update ---> ${path}" >> ${FAIL_LOG}
    echo "${result}" >> ${FAIL_LOG}
    echo >> ${FAIL_LOG}
  fi

  echo

done
