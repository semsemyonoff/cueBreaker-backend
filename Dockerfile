FROM python:3.14-alpine AS shntool-builder

RUN apk add --no-cache build-base curl
RUN curl -fsSL http://shnutils.freeshell.org/shntool/dist/src/shntool-3.0.10.tar.gz \
    | tar xz -C /tmp \
 && cd /tmp/shntool-3.0.10 \
 && curl -fsSL -o config.guess 'https://raw.githubusercontent.com/gcc-mirror/gcc/master/config.guess' \
 && curl -fsSL -o config.sub   'https://raw.githubusercontent.com/gcc-mirror/gcc/master/config.sub' \
 && chmod +x config.guess config.sub \
 && ./configure --prefix=/usr CFLAGS="-O2 -std=gnu11" \
 && make -j$(nproc) \
 && make install DESTDIR=/out

FROM python:3.14-alpine

COPY --from=shntool-builder /out/usr/bin/shntool /usr/bin/shntool
RUN ln -s shntool /usr/bin/shnsplit

RUN apk add --no-cache \
    cuetools \
    flac

RUN pip install --no-cache-dir flask gunicorn

WORKDIR /app
COPY app.py .
COPY templates/ templates/
COPY static/ static/

EXPOSE 5000

CMD ["gunicorn", "-b", "0.0.0.0:5000", "-w", "2", "--threads", "4", "--access-logfile", "-", "--error-logfile", "-", "app:app"]
