#!/bin/bash

# TODO automate
GO_VERSION=1.21.13

FAIL_LOG="go-get-upgrade.fail.log"
if [ -f ${FAIL_LOG} ]; then
  rm ${FAIL_LOG}
fi

while read dep
do
  path=$(echo "${dep}" | jq -r .Path)
  if [ -z "${path}" ]; then
    echo "  Error: Cannot process ${dep}"
    echo
    continue
  fi

  main=$(echo "${dep}" | jq -r .Main)
  if [ "${main}" == "true" ]; then
    # Do not update ourselves
    echo
    continue
  fi

  update=$(echo "${dep}" | jq -r '.Update | select( . != null )')
  if [ -z "${update}" ]; then
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
  curr=$(echo "${dep}" | jq -r '.Version | select( . != null )')
  new=$(echo "${dep}" | jq -r '.Update.Version | select( . != null )')

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

done < <(cat gomod-list.json | jq -c '.[]')
