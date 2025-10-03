from typing import List
from fastapi import FastAPI
from pydantic import BaseModel

app = FastAPI()


class RerankRequest(BaseModel):
    query: str
    docs: List[str]


class RerankResponse(BaseModel):
    order: List[int]


@app.get("/healthz")
def healthz():
    return {"status": "ok"}


@app.post("/rerank", response_model=RerankResponse)
def rerank(req: RerankRequest):
    # Trivial identity ranking placeholder
    return RerankResponse(order=list(range(len(req.docs))))

# To run: uvicorn reranker.app:app --reload --host 0.0.0.0 --port 8000

