#!/usr/bin/env bash

set -euo pipefail

if [[ ! ${VERSION} =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	echo "release version must be a stable vMAJOR.MINOR.PATCH tag: ${VERSION}" >&2
	exit 1
fi

release_dir="${BUILD_DIR}/release"
rm -rf "${release_dir}"
mkdir -p "${release_dir}"

for target in ${REL_TARGET}; do
	os=${target%%-*}
	arch=${target#*-}
	name="rapidgo-${VERSION}-${target}"
	binary="${release_dir}/${name}"
	if [[ ${os} == windows ]]; then
		binary+=.exe
	fi
	GOOS="${os}" GOARCH="${arch}" CGO_ENABLED=0 "${GO}" build ${GOFLAGS} \
		-ldflags "${LDFLAGS}" -o "${binary}" ./cmd/rapidgo
	if [[ ${os} == windows ]]; then
		(cd "${release_dir}" && zip -q "${name}.zip" "${name}.exe")
	else
		gzip -n "${binary}"
	fi
	rm -f "${binary}"
	echo "Built ${target}"
done

cp LICENSE "${release_dir}/LICENSE"
(cd "${release_dir}" && shasum -a 512 * > checksum-sha512.txt)

previous=$(git describe --tags --abbrev=0 HEAD^ 2>/dev/null || true)
if [[ -n ${previous} ]]; then
	printf 'Changes since %s:\n\n' "${previous}" > "${BUILD_DIR}/CHANGELOG"
	git log --pretty=format:'* %h %s' "${previous}..HEAD" >> "${BUILD_DIR}/CHANGELOG"
else
	printf 'Changes in %s:\n\n' "${VERSION}" > "${BUILD_DIR}/CHANGELOG"
	git log --pretty=format:'* %h %s' HEAD >> "${BUILD_DIR}/CHANGELOG"
fi
printf '\n' >> "${BUILD_DIR}/CHANGELOG"
