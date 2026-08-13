ARG KALI_BASE_IMAGE=docker.io/kalilinux/kali-rolling:latest

FROM ${KALI_BASE_IMAGE}

ARG AGENT_BROWSER_VERSION=latest

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
		make \
		pkg-config \
		libsqlite3-dev \
        ca-certificates \
        chromium \
        curl \
        default-jdk-headless \
        dnsutils \
        git \
        golang-go \
        jq \
		sqlite3 \
		tree \
		rsync \
		file \
		less \
		shellcheck \
		httpie \
        netcat-openbsd \
        nmap \
        nodejs \
        npm \
        openssh-client \
        procps \
        python3 \
        python3-pip \
        python3-venv \
        ripgrep \
        unzip \
		zip \
		tar \
        whois \
    && ln -sf /usr/bin/python3 /usr/local/bin/python \
    && ln -sf /usr/bin/pip3 /usr/local/bin/pip \
    && npm install --global \
        "agent-browser@${AGENT_BROWSER_VERSION}" \
		pnpm \
		yarn \
    && if [ "$(dpkg --print-architecture)" = "amd64" ]; then agent-browser install; fi \
    && npm cache clean --force \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* /tmp/* \
    && groupadd --gid 10001 aegis \
    && useradd --uid 10001 --gid 10001 --create-home --shell /bin/bash aegis \
    && mkdir -p /workspace /go \
    && chown -R 10001:10001 /workspace /go

WORKDIR /workspace

ENV NODE_ENV=production
ENV GOPATH=/go
ENV AGENT_BROWSER_EXECUTABLE_PATH=/usr/bin/chromium
ENV PATH=/go/bin:${PATH}

USER 10001:10001

CMD ["sleep", "infinity"]
