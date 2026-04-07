FROM python:3.10.19-trixie

COPY parser.py /parser.py
COPY parser.sh /parser.sh
RUN chmod 777 /parser.sh
RUN apt-get update && apt-get install -y curl git tini

RUN curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
RUN chmod 700 get_helm.sh
RUN ./get_helm.sh

RUN groupadd -g 999 argocd
RUN useradd -m -u 999 -g 999 -s /bin/bash argocd

COPY clone.sh /clone.sh
RUN chmod +x /clone.sh

ENTRYPOINT ["tini", "--"]