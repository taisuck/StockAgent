# 실행 단계
FROM alpine:3.20

RUN addgroup -S -g 10001 app \
 && adduser -S -u 10001 -G app app \
 && mkdir -p /app/logs \
 && chown -R app:app /app

ENV TZ=Asia/Seoul

WORKDIR /app

COPY ./output/stockagent /app/stockagent
COPY ./conf/config.yml /app/config.yml

USER app

ENTRYPOINT ["/app/stockagent"]