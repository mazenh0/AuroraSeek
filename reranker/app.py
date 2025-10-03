from fastapi import FastAPI
from pydantic import BaseModel
from typing import List, Dict
import uvicorn
import math

app = FastAPI()

class Candidate(BaseModel):
    id: str
    title: str
    body: str

class RerankIn(BaseModel):
    query: str
    candidates: List[Candidate]

class RerankOut(BaseModel):
    order: List[str]

# Tiny bag-of-words "embedding"
def embed(text: str) -> Dict[str, float]:
    v = {}
    for tok in text.lower().split():
        v[tok] = v.get(tok, 0.0) + 1.0
    # l2 normalize
    norm = math.sqrt(sum(t*t for t in v.values())) or 1.0
    for k in v:
        v[k] /= norm
    return v

def cosine(a: Dict[str,float], b: Dict[str,float]) -> float:
    keys = a.keys() & b.keys()
    return sum(a[k]*b[k] for k in keys)

@app.post("/rerank", response_model=RerankOut)
async def rerank(inp: RerankIn):
    qv = embed(inp.query)
    scored = []
    for c in inp.candidates:
        dv = embed(c.title + " " + c.body)
        scored.append((c.id, cosine(qv, dv)))
    scored.sort(key=lambda x: x[1], reverse=True)
    ordered = [sid for sid,_ in scored]
    return RerankOut(order=ordered)

if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8000)
