# ---------- Base Stage ----------
FROM python:3.11-slim AS base
WORKDIR /app
ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt
COPY src/ ./src/

# ---------- Development Stage ----------
FROM base AS dev
RUN apt-get update && apt-get install -y \
    curl \
    bash \
    git \
    build-essential \
    libssl-dev \
    vim \
    && rm -rf /var/lib/apt/lists/*
RUN pip install debugpy
EXPOSE 5678
CMD ["sleep", "infinity"]

# ---------- Production Stage ----------
FROM base AS prod
CMD ["python", "-m", "src.main"]
