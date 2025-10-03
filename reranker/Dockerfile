FROM python:3.11-slim

WORKDIR /app
COPY app.py ./
RUN pip install fastapi==0.115.0 uvicorn==0.30.6

EXPOSE 8000
CMD ["python", "app.py"]
