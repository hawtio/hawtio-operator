#!/bin/bash

# ---------------------------------------------------------------------------
# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ---------------------------------------------------------------------------


check_env_var() {
  if [ -z "${2}" ]; then
    echo "Error: ${1} env var not defined"
    exit 1
  fi
}

check_env_var "BUNDLE_INDEX" ${BUNDLE_INDEX}
check_env_var "INDEX_DIR" ${INDEX_DIR}
check_env_var "PACKAGE" ${PACKAGE}
check_env_var "OPM" ${OPM}
check_env_var "YQ" ${YQ}
check_env_var "BUNDLE_IMAGE" ${BUNDLE_IMAGE}
check_env_var "CSV_NAME" ${CSV_NAME}
check_env_var "CHANNELS" ${CHANNELS}
check_env_var "CONTAINER_BUILDER" ${CONTAINER_BUILDER}

NEW_BUNDLE_YAML=${INDEX_DIR}/${CSV_NAME}-bundle.yaml
INDEX_BASE_YAML=${INDEX_DIR}/bundles.yaml

echo "=== Checking OPM command ..."
if ! command -v ${OPM} &> /dev/null
then
  echo "Error: opm is not available. Was OPM env var defined correctly: ${OPM}"
  exit 1
fi

echo "=== Checking container-builder ..."
if ! command -v ${CONTAINER_BUILDER} &> /dev/null
then
  echo "Error: ${CONTAINER_BUILDER} is not available. Was CONTAINER_BUILDER env var defined correctly: ${CONTAINER_BUILDER}"
  exit 1
fi

echo "=== Checking IMAGE variable for double bundle suffix..."
if [[ "${BUNDLE_IMAGE}" =~ .*-bundle-bundle.* ]]; then
  echo "Error: IMAGE variable from Makefile should NOT include the -bundle prefix!"
  exit 1
fi

echo "=== Checking CSV Replacement and Skips ..."
if [ -n "${CSV_REPLACES}" ] && [ -n "${CSV_SKIPS}" ]; then
  echo
  echo "Both CSV_REPLACES and CSV_SKIPS have been specified."
  while [ -z "${brs}" ]; do
    read -p "Do you wish to include both (b), ignore 'replaces' (r) or ignore 'skips' (s): " brs
    case ${brs} in
        [Bb]* )
          echo "... including both"
          echo
          ;;
        [Rr]* )
          echo ".. ignoring 'replaces'"
          echo
          CSV_REPLACES=""
          ;;
        [Ss]* )
          echo ".. ignoring 'skips'"
          echo
          CSV_SKIPS=""
          ;;
        * )
          echo "Please answer b, r or s."
          echo
          ;;
    esac
  done
fi

echo "=== Checking for old Dockerfile"
if [ -f "${INDEX_DIR}.Dockerfile" ]; then
  rm -f "${INDEX_DIR}.Dockerfile"
fi

echo "=== Removing old index directory"
if [ -d "${INDEX_DIR}" ]; then
  rm -rf "${INDEX_DIR}"
fi
mkdir -p "${INDEX_DIR}"

echo "=== Checking yq version"
YQ_VERSION=$(yq --version | sed 's/.*v\([0-9]\).*/\1/')
if [ ${YQ_VERSION} -lt 4 ]; then
  echo "Error: yq version should be at least v4"
  exit 1
fi

echo "=== Checking index base ..."
if [ ! -f ${INDEX_BASE_YAML} ]; then
	# Pull the latest version of the catalog index image
  echo "=== Pulling bundle-index image ${BUNDLE_INDEX} ..."
	${CONTAINER_BUILDER} pull ${BUNDLE_INDEX}
	if [ $? != 0 ]; then
    echo "Error: failed to pull latest version of bundle catalog index image"
    exit 1
  fi
  echo "=== Calling opm render on ${BUNDLE_INDEX} ..."
  ${OPM} render ${BUNDLE_INDEX} -o yaml > ${INDEX_BASE_YAML}
  if [ $? != 0 ]; then
    echo "Error: failed to render the base catalog"
    exit 1
  fi

  #
  # Filter the base index to *only* include our package.
  # This avoids validation errors from other, unrelated packages.
  #
  echo "=== Filtering base index to include ONLY '${PACKAGE}' manifests..."
  temp_index_file=$(mktemp ${INDEX_DIR}/temp-index-XXX.yaml)
  trap 'rm -f ${temp_index_file}' EXIT

  ${YQ} eval ". | select( \
    (.schema == \"olm.package\" and .name == \"${PACKAGE}\") or \
    (.schema == \"olm.channel\" and .package == \"${PACKAGE}\") or \
    (.schema == \"olm.bundle\"  and .package == \"${PACKAGE}\") \
  )" ${INDEX_BASE_YAML} > ${temp_index_file}
  if [ $? != 0 ]; then
    echo "ERROR: Failed to filter base index for ${PACKAGE}"
    exit 1
  fi

  # Now, replace the original index file with filtered one
  echo "=== Replacing ${INDEX_BASE_YAML} with filtered version ..."
  mv ${temp_index_file} ${INDEX_BASE_YAML}
  echo "=== Base index successfully filtered."
fi

if [ ! -f ${NEW_BUNDLE_YAML} ]; then
  echo "=== Calling opm render on ${BUNDLE_IMAGE} ..."
  ${OPM} render -o yaml ${BUNDLE_IMAGE} > ${NEW_BUNDLE_YAML}
  if [ $? != 0 ]; then
    echo "Error: failed to render the ${PACKAGE} bundle catalog"
    exit 1
  fi

  #
  # Determine whether to add a package schema or not
  # Applicable to only brand-new products
  #
  echo "=== Determing whether to add a package schema or not ..."
  RESULT=$(${YQ} eval ". | select(.schema == \"olm.package\" and .name == \"${PACKAGE}\") | .name" ${INDEX_BASE_YAML})
  if [ "${RESULT}" != "${PACKAGE}" ]; then
    echo "Cannot find package entry in catalog. Adding package to ${NEW_BUNDLE_YAML} ..."

    object="{ \"defaultChannel\": \"latest\", \"name\": \"${PACKAGE}\", \"schema\": \"olm.package\" }"
    package_file=$(mktemp ${PACKAGE}-package-XXX.yaml)
    trap 'rm -f ${package_file}' EXIT
    ${YQ} -n eval "${object}" > ${package_file}

    echo "---" >> ${NEW_BUNDLE_YAML}
    cat ${package_file} >> ${NEW_BUNDLE_YAML}
  fi
fi

#
# Extract ALL existing channels into their own standalone FBC files first!
#
echo "=== Extracting all existing channels to standalone FBC files ..."
# --no-doc stops the --- being included
EXISTING_CHANNELS=$(${YQ} --no-doc eval ". | select(.package == \"${PACKAGE}\" and .schema == \"olm.channel\") | .name" ${INDEX_BASE_YAML})
if [ -n "${EXISTING_CHANNELS}" ]; then
  for existing_channel in ${EXISTING_CHANNELS}; do
    echo "  Extracting existing channel: ${existing_channel} ..."
    CHANNEL_FILE="${INDEX_DIR}/${PACKAGE}-channel-${existing_channel}.yaml"
    ${YQ} eval ". | select(.package == \"${PACKAGE}\" and .schema == \"olm.channel\" and .name == \"${existing_channel}\")" ${INDEX_BASE_YAML} > "${CHANNEL_FILE}"
  done
fi

#
# Filter out the channels in the bundles file
#
echo "=== Filter out the channels in the bundles file ..."
${YQ} -i eval ". | select(.package != \"${PACKAGE}\" or .schema != \"olm.channel\")" ${INDEX_BASE_YAML}
if [ $? != 0 ]; then
  echo "ERROR: Failed to remove package channel entries from bundles catalog"
  exit 1
fi

#
# Create or insert entries of new version of operator
# into channels files
#
echo "=== Iterating over new operator channels (new channel or insert into existing one) ..."
OLD_IFS=${IFS}
IFS=','
#Read the split words into an array based on comma delimiter
read -r -a CHANNEL_ARR <<< "${CHANNELS}"
IFS=${OLD_IFS}

for channel in "${CHANNEL_ARR[@]}";
do
  # Define a dedicated file for this single channel
  CHANNEL_FILE="${INDEX_DIR}/${PACKAGE}-channel-${channel}.yaml"

  # Build the new entry object
  entry="{ \"name\": \"${CSV_NAME}\""
  if [ -n "${CSV_REPLACES}" ]; then
    entry="${entry}, \"replaces\": \"${CSV_REPLACES}\""
  fi
  if [ -n "${CSV_SKIPS}" ]; then
    entry="${entry}, \"skipRange\": \"${CSV_SKIPS}\""
  fi
  entry="${entry} }"
  yaml_entry=$(echo "${entry}" | ${YQ} eval -P)

  # If the file is empty, this is a brand new channel
  if [ ! -s "${CHANNEL_FILE}" ]; then
    echo "Creating new standalone channel file for ${channel} ..."
    object="{ \"schema\": \"olm.channel\", \"package\": \"${PACKAGE}\", \"name\": \"${channel}\", \"entries\": [${entry}] }"
    echo "${object}" | ${YQ} eval -P > "${CHANNEL_FILE}"
  else
    # The channel exists. Append using yq
    echo "Updating standalone channel file for ${channel} ..."
    ${YQ} -i eval ".entries += [${entry}]" ${CHANNEL_FILE}
  fi
done

echo "=== Validating index ... "
STATUS=$(${OPM} validate ${INDEX_DIR} 2>&1)
if [ $? != 0 ]; then
  echo "Failed"
  echo "Error: ${STATUS}"
  exit 1
else
  echo "OK"
fi

echo "=== Generating catalog dockerfile ... "
STATUS=$(${OPM} generate dockerfile ${INDEX_DIR} 2>&1)
if [ $? != 0 ]; then
  echo "Failed"
  echo "Error: ${STATUS}"
  exit 1
else
  echo "OK"
fi

echo "=== Setting file permissions on index directory ..."
chmod -R a+r "${INDEX_DIR}"

echo "=== Building catalog image ... "
BUNDLE_INDEX_IMAGE="${BUNDLE_IMAGE%:*}-index":"${BUNDLE_IMAGE#*:}"
STATUS=$(${CONTAINER_BUILDER} build . -f ${INDEX_DIR}.Dockerfile -t ${BUNDLE_INDEX_IMAGE} 2>&1)
if [ $? != 0 ]; then
  echo "Failed"
  echo "Error: ${STATUS}"
  exit 1
else
  echo "OK"
  echo "Index image ${BUNDLE_INDEX_IMAGE} can be pushed"
fi
