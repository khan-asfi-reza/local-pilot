// area.c - compute areas of simple shapes
#include <stdio.h>

/* returns the area of a rectangle */
double rect_area(double w, double h) {
    return w * h; // width times height
}

/* returns the area of a circle */
double circle_area(double r) {
    return 3.14159 * r * r; // pi times r squared
}

int main(void) {
    printf("%f\n", rect_area(3.0, 4.0)); // expect 12
    printf("%f\n", circle_area(2.0));
    return 0;
}
