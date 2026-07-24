# Copyright © 2026 Christian Fritz <mail@chr-fritz.de>
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM busybox:stable-glibc
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/speedwire-exporter /usr/bin/
COPY defaultConfig.yaml /etc/speedwire-exporter/config.yaml
EXPOSE 8080/tcp
VOLUME /etc/speedwire-exporter
RUN  addgroup -g 1337 speedwire-exporter \
     && adduser -s /usr/sbin/nologin -G speedwire-exporter -D -H -u 1337 speedwire-exporter
USER speedwire-exporter
ENTRYPOINT ["/usr/bin/speedwire-exporter"]
CMD ["run", "--config","/etc/speedwire-exporter/config.yaml"]
