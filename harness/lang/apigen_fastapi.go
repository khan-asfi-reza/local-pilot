package lang

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FastAPI builder. Emits SQLAlchemy models (which define the schema — the scaffold's
// init_db() runs Base.metadata.create_all at boot, so there are NO migration files)
// plus Pydantic In/Out schemas, and auto-included CRUD routers. Each route imports
// its model module, so the table is registered on Base before create_all runs. Auth
// guards are left to the model to add inside the LOGIC blocks (the guard name lives
// in app/auth.py), so a generated route always boots.

func init() {
	registerBuilder(&Builder{
		Framework: "fastapi",
		Models:    fastapiModels,
		Routes:    fastapiRoutes,
	})
}

func fastapiModels(spec BuildSpec) []GenFile {
	var files []GenFile
	for _, e := range spec.Entities {
		files = append(files, GenFile{
			Path:    filepath.Join("app", "models", e.Name+".py"),
			Content: fastapiModelPy(e),
		})
	}
	return files
}

func pyBool(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

func fastapiColLine(f BuildField) string {
	var parts []string
	if f.Ref != "" {
		rt, rc := refTarget(f.Ref)
		parts = append(parts, "Integer", fmt.Sprintf("ForeignKey(%q)", rt+"."+rc))
	} else {
		parts = append(parts, pySAType(f.Type))
	}
	parts = append(parts, "nullable="+pyBool(f.Nullable))
	if f.Unique {
		parts = append(parts, "unique=True")
	}
	if f.Index || f.Ref != "" {
		parts = append(parts, "index=True")
	}
	return fmt.Sprintf("    %s = Column(%s)", f.Name, strings.Join(parts, ", "))
}

func fastapiModelPy(e BuildEntity) string {
	cn := title(e.Name)
	var b strings.Builder
	b.WriteString("from sqlalchemy import Column, Integer, String, Boolean, Float, Date, DateTime, Text, ForeignKey\n")
	b.WriteString("from sqlalchemy.sql import func\n")
	b.WriteString("from pydantic import BaseModel\n\n")
	b.WriteString("from app.db import Base\n\n\n")

	fmt.Fprintf(&b, "class %s(Base):\n", cn)
	fmt.Fprintf(&b, "    __tablename__ = %q\n\n", e.Table)
	b.WriteString("    id = Column(Integer, primary_key=True)\n")
	for _, f := range e.Fields {
		b.WriteString(fastapiColLine(f) + "\n")
	}
	b.WriteString("    created_at = Column(DateTime(timezone=True), server_default=func.now())\n\n\n")

	fmt.Fprintf(&b, "class %sIn(BaseModel):\n", cn)
	if len(e.Fields) == 0 {
		b.WriteString("    pass\n\n\n")
	} else {
		for _, f := range e.Fields {
			line := "    " + f.Name + ": " + pyFieldType(f.Type)
			if f.Nullable {
				line += " | None = None"
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n\n")
	}

	fmt.Fprintf(&b, "class %sOut(%sIn):\n", cn, cn)
	b.WriteString("    id: int\n\n")
	b.WriteString("    class Config:\n        from_attributes = True\n")
	return b.String()
}

func fastapiRoutes(spec BuildSpec) ([]GenFile, []string) {
	var files []GenFile
	var blanks []string
	for _, e := range spec.Entities {
		files = append(files, GenFile{
			Path:    filepath.Join("app", "routes", e.Table+".py"),
			Content: fastapiRoutePy(e),
		})
		blanks = append(blanks, fmt.Sprintf("app/routes/%s.py — list/get/create/update/delete for %s (fill each LOGIC block)", e.Table, e.Name))
	}
	return files, blanks
}

func fastapiRoutePy(e BuildEntity) string {
	cn := title(e.Name)
	t := e.Table
	var b strings.Builder
	b.WriteString("from fastapi import APIRouter, Depends, HTTPException\n")
	b.WriteString("from sqlalchemy.orm import Session\n\n")
	b.WriteString("from app.db import get_db\n")
	fmt.Fprintf(&b, "from app.models.%s import %s, %sIn, %sOut\n\n", e.Name, cn, cn, cn)
	fmt.Fprintf(&b, "# Auto-included at /api/%s. CRUD for %s. Fill ONLY the code inside each marked\n", t, e.Name)
	b.WriteString("# LOGIC block (between its open and close comment); leave everything else as-is.\n")
	fmt.Fprintf(&b, "router = APIRouter(prefix=%q, tags=[%q])\n\n\n", "/api/"+t, t)

	// LIST
	fmt.Fprintf(&b, "@router.get(\"\", response_model=list[%sOut])\n", cn)
	fmt.Fprintf(&b, "def list_%s(db: Session = Depends(get_db)):\n", t)
	fmt.Fprintf(&b, "    # === LOGIC:list %s ===\n", t)
	b.WriteString("    # Returns all rows. Add filtering / sorting / pagination / ownership scoping here.\n")
	fmt.Fprintf(&b, "    return db.query(%s).order_by(%s.id.desc()).all()\n", cn, cn)
	b.WriteString("    # === END LOGIC ===\n\n\n")

	// GET
	fmt.Fprintf(&b, "@router.get(\"/{item_id}\", response_model=%sOut)\n", cn)
	fmt.Fprintf(&b, "def get_%s(item_id: int, db: Session = Depends(get_db)):\n", e.Name)
	fmt.Fprintf(&b, "    # === LOGIC:get %s ===\n", e.Name)
	fmt.Fprintf(&b, "    obj = db.get(%s, item_id)\n", cn)
	b.WriteString("    if obj is None:\n        raise HTTPException(status_code=404, detail=\"not found\")\n")
	b.WriteString("    return obj\n")
	b.WriteString("    # === END LOGIC ===\n\n\n")

	// CREATE
	fmt.Fprintf(&b, "@router.post(\"\", response_model=%sOut, status_code=201)\n", cn)
	fmt.Fprintf(&b, "def create_%s(payload: %sIn, db: Session = Depends(get_db)):\n", e.Name, cn)
	fmt.Fprintf(&b, "    # === LOGIC:create %s ===\n", e.Name)
	b.WriteString("    # Validate input and set any owner_id/user_id from the authed user here.\n")
	fmt.Fprintf(&b, "    obj = %s(**payload.model_dump())\n", cn)
	b.WriteString("    db.add(obj)\n    db.commit()\n    db.refresh(obj)\n    return obj\n")
	b.WriteString("    # === END LOGIC ===\n\n\n")

	// UPDATE
	fmt.Fprintf(&b, "@router.put(\"/{item_id}\", response_model=%sOut)\n", cn)
	fmt.Fprintf(&b, "def update_%s(item_id: int, payload: %sIn, db: Session = Depends(get_db)):\n", e.Name, cn)
	fmt.Fprintf(&b, "    # === LOGIC:update %s ===\n", e.Name)
	fmt.Fprintf(&b, "    obj = db.get(%s, item_id)\n", cn)
	b.WriteString("    if obj is None:\n        raise HTTPException(status_code=404, detail=\"not found\")\n")
	b.WriteString("    for key, value in payload.model_dump(exclude_unset=True).items():\n        setattr(obj, key, value)\n")
	b.WriteString("    db.commit()\n    db.refresh(obj)\n    return obj\n")
	b.WriteString("    # === END LOGIC ===\n\n\n")

	// DELETE
	b.WriteString("@router.delete(\"/{item_id}\", status_code=204)\n")
	fmt.Fprintf(&b, "def delete_%s(item_id: int, db: Session = Depends(get_db)):\n", e.Name)
	fmt.Fprintf(&b, "    # === LOGIC:delete %s ===\n", e.Name)
	fmt.Fprintf(&b, "    obj = db.get(%s, item_id)\n", cn)
	b.WriteString("    if obj is not None:\n        db.delete(obj)\n        db.commit()\n")
	b.WriteString("    # === END LOGIC ===\n")
	return b.String()
}
